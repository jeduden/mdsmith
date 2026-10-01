// Package refactor is the workspace refactor engine shared by the LSP
// server, the `mdsmith move` / `mdsmith rename` CLI commands, and the
// WASM engine. It answers one question: given a workspace and an
// identity change — a file's path, a heading's slug, or a
// link-reference label — what edits carry that change through every
// reference, and what file operation (if any) does the host perform?
//
// Every operation returns a neutral Plan: per-file Edits keyed by
// output target (a CLI path or an LSP document URI) plus an optional
// FileOp the host executes. The engine speaks no LSP wire types and
// its planners never touch the filesystem — there are no subprocesses
// under GOOS=js GOARCH=wasm — so callers adapt the neutral values to
// their surface and run any FileOp themselves. ApplyEdits is the
// matching pure in-memory splice a host can use to turn a file's Edits
// into rewritten bytes; FileOp.Execute (non-wasm only) is the one
// helper that touches disk.
package refactor

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// Position is a zero-based line / UTF-16 character offset, matching
// the LSP coordinate model the engine already computes. It is a
// rename-owned type, not an LSP wire type.
type Position struct {
	Line      int
	Character int
}

// Range is a half-open [Start, End) span within one file.
type Range struct {
	Start Position
	End   Position
}

// Edit replaces the text in Range with NewText.
type Edit struct {
	Range   Range
	NewText string
}

// ErrEmptyLabel is returned when a link-reference rename is asked to
// produce an empty label.
var ErrEmptyLabel = fmt.Errorf("label cannot be empty")

// InvalidLabelRuneError reports the first rune that would make the
// rewritten `[label]: …` line unparsable.
type InvalidLabelRuneError struct{ Rune rune }

func (e InvalidLabelRuneError) Error() string {
	if e.Rune == '\\' {
		return "label cannot end with an unescaped backslash"
	}
	return fmt.Sprintf("label cannot contain %q", e.Rune)
}

// LabelConflictError reports that the new label collides with another
// reference definition already in the file. Conflict carries the
// colliding label's original (non-normalized) spelling.
type LabelConflictError struct{ Conflict string }

func (e LabelConflictError) Error() string {
	return "rename would collide with link reference [" + e.Conflict + "]"
}

// LinkRef computes a Plan that renames a link-reference label from
// oldLabel to newName. The rewrite is file-local — the `[label]: url`
// definition plus every `[text][label]` and shortcut `[label]` use —
// so every edit groups under fileKey (the CLI file path or LSP
// document URI the caller keys the source under) and Plan.FileOp is
// always nil. oldLabel is normalized internally (CommonMark link-label
// normalization: lowercase, whitespace collapsed) so callers may pass
// raw label text or a pre-normalized form.
//
// Returns ErrEmptyLabel, an InvalidLabelRuneError, or a
// LabelConflictError with a zero Plan and no edit when the rename is
// unsafe, so callers can surface the failure before applying.
func LinkRef(fileKey string, source []byte, oldLabel, newName string) (Plan, error) {
	oldLabel = NormalizedLabel([]byte(oldLabel))
	if strings.TrimSpace(newName) == "" {
		return Plan{}, ErrEmptyLabel
	}
	if invalid := invalidLinkRefRune(newName); invalid != 0 {
		return Plan{}, InvalidLabelRuneError{Rune: invalid}
	}
	// A rename that keeps the same normalized label (e.g. "docs api"
	// → "Docs API") is allowed — it refreshes casing/spacing across
	// the def and every use. labelConflict matches on the normalized
	// form so such a rename never collides with itself.
	newLabel := NormalizedLabel([]byte(newName))
	if conflict := labelConflict(source, oldLabel, newLabel); conflict != "" {
		return Plan{}, LabelConflictError{Conflict: conflict}
	}
	edits := linkRefEdits(source, oldLabel, newName)
	return Plan{Edits: map[string][]Edit{fileKey: edits}}, nil
}

// HasLinkRef reports whether source defines a reference definition
// whose normalized label matches label (CommonMark link-label
// normalization). The `rename` CLI uses it to auto-detect a link-ref
// rename when `--as` is omitted. Def-shaped lines inside code blocks
// or paragraph continuations are excluded, matching LinkRef.
func HasLinkRef(source []byte, label string) bool {
	want := NormalizedLabel([]byte(label))
	body, _ := bodyAndFMOffset(source)
	for _, m := range validRefDefMatches(body) {
		if m.normLabel == want {
			return true
		}
	}
	return false
}

// ValidRefDefBodyLines reports the body-line indices that hold a real
// reference definition goldmark accepted (not a code-block
// look-alike). The LSP prepare-rename gate consults it so the rename
// UI never surfaces on a `[label]: url`-shaped code sample.
func ValidRefDefBodyLines(body []byte) map[int]struct{} {
	out := map[int]struct{}{}
	for _, m := range validRefDefMatches(body) {
		out[m.bodyLine] = struct{}{}
	}
	return out
}

// BodyAndFMOffset splits source into its body and the line count the
// front matter contributed, so callers translate source-line
// coordinates into the body coordinates the engine parses in.
func BodyAndFMOffset(source []byte) ([]byte, int) {
	return bodyAndFMOffset(source)
}

// invalidLinkRefRune returns the first rune in s that would make the
// resulting `[label]: …` line unparsable, or 0 when s is safe.
//
// Newlines force the label run onto a second line where the def no
// longer parses. `]` ends the label early. `[` is technically
// escapable, but emitting a raw `[` would still confuse most
// CommonMark renderers and the ref-def regex, so both bracket forms
// are rejected outright rather than auto-escaped. A trailing
// unescaped backslash (an odd-length run at the end) escapes the
// closing `]`, so it is reported as '\\'.
func invalidLinkRefRune(s string) rune {
	for _, r := range s {
		switch r {
		case '\n', '\r', '[', ']':
			return r
		}
	}
	trailing := len(s) - len(strings.TrimRight(s, `\`))
	if trailing%2 == 1 {
		return '\\'
	}
	return 0
}

// NormalizedLabel returns the CommonMark-normalized form of a link
// label — lowercase with internal whitespace collapsed — by delegating
// to goldmark's util.ToLinkReference.
func NormalizedLabel(b []byte) string {
	return string(util.ToLinkReference(b))
}

// labelConflict returns the conflicting label's original casing when
// newLabel matches a reference definition other than the one being
// renamed, or "" when there is no conflict. The scan filters regex
// matches through goldmark's parser context so a `[label]: url`-
// shaped line inside a fenced code block or PI body never counts as
// a real def.
func labelConflict(source []byte, oldLabel, newLabel string) string {
	body, _ := bodyAndFMOffset(source)
	for _, m := range validRefDefMatches(body) {
		if m.normLabel == oldLabel {
			continue
		}
		if m.normLabel == newLabel {
			return m.rawLabel
		}
	}
	return ""
}

// validRefDefMatch is one source-validated reference definition
// position. Validation goes through goldmark's parser context so
// matches inside code / PI blocks drop out.
type validRefDefMatch struct {
	bodyLine  int
	rawLabel  string
	normLabel string
	matchIdx  []int
}

// validRefDefMatches returns the ref-def regex matches that fall
// outside any AST node. Real reference definitions never appear in
// the AST (goldmark consumes them into the parser context), so a
// regex hit on a line goldmark did not tuck into a block is a real
// def. This drops paragraph-continuation lookalikes, code-block
// content, and PI bodies in one pass.
func validRefDefMatches(body []byte) []validRefDefMatch {
	if !bytes.Contains(body, []byte("]:")) {
		return nil
	}
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	consumed := contentBlockLines(root, body)
	var out []validRefDefMatch
	for _, m := range index.RefDefRegexpMatches(body) {
		bodyLine := lineOfBodyOffset(body, m[2])
		if _, ok := consumed[bodyLine]; ok {
			continue
		}
		raw := body[m[2]:m[3]]
		norm := NormalizedLabel(raw)
		out = append(out, validRefDefMatch{
			bodyLine:  bodyLine,
			rawLabel:  string(raw),
			normLabel: norm,
			matchIdx:  m,
		})
	}
	return out
}

// contentBlockLines returns the set of body-line numbers goldmark
// consumed into any AST node. Real reference definitions live in
// parser.Context, never the AST, so any line covered by an AST node
// is by definition not a def. The Document root and
// LinkReferenceDefinition nodes are skipped: the former spans the
// whole buffer, the latter IS the line a real def lives on.
func contentBlockLines(root ast.Node, body []byte) map[int]struct{} {
	out := map[int]struct{}{}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.Document, *ast.LinkReferenceDefinition:
			return ast.WalkContinue, nil
		}
		ls := n.Lines()
		for i := 0; i < ls.Len(); i++ {
			seg := ls.At(i)
			out[lineOfBodyOffset(body, seg.Start)] = struct{}{}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// linkRefEdits walks the source for the def line and every
// reference-style use of oldLabel (full and shortcut), returning one
// Edit per match.
func linkRefEdits(source []byte, oldLabel, newName string) []Edit {
	body, fmOffset := bodyAndFMOffset(source)
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	lines := splitLines(source)
	out := make([]Edit, 0, 8)
	out = append(out, refDefEditsInBody(body, lines, fmOffset, oldLabel, newName)...)
	out = append(out, refUseEditsInBody(root, body, lines, fmOffset, oldLabel, newName)...)
	return out
}

// refDefEditsInBody finds the `[label]: url` line(s) for oldLabel and
// emits one Edit per match. A file may legally carry duplicate def
// lines (goldmark only resolves the first); all are rewritten so the
// file stays internally consistent. Filtering goes through
// validRefDefMatches so a def-shaped line inside a code block is not
// rewritten.
func refDefEditsInBody(
	body []byte, lines [][]byte, fmOffset int,
	oldLabel, newName string,
) []Edit {
	var out []Edit
	for _, m := range validRefDefMatches(body) {
		if m.normLabel != oldLabel {
			continue
		}
		fileLine := m.bodyLine + fmOffset
		if fileLine-1 >= len(lines) {
			continue
		}
		row := lines[fileLine-1]
		bracket := RefDefBracketBytes(row)
		startCh := mdtext.UTF16FromByteOffset(row, bracket[0])
		endCh := mdtext.UTF16FromByteOffset(row, bracket[1])
		out = append(out, Edit{
			Range: Range{
				Start: Position{Line: fileLine - 1, Character: startCh},
				End:   Position{Line: fileLine - 1, Character: endCh},
			},
			NewText: newName,
		})
	}
	return out
}

// refUseEditsInBody walks the AST for ast.Link and ast.Image nodes
// whose Reference matches oldLabel and emits one Edit per use.
func refUseEditsInBody(
	root ast.Node, body []byte, lines [][]byte, fmOffset int,
	oldLabel, newName string,
) []Edit {
	idx := newBodyLineIndex(body)
	var out []Edit
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		ref := referenceOf(n)
		if ref == nil || NormalizedLabel(ref.Value) != oldLabel {
			return ast.WalkContinue, nil
		}
		edit, ok := refUseEdit(n, ref, body, lines, fmOffset, newName, idx)
		if ok {
			out = append(out, edit)
		}
		return ast.WalkContinue, nil
	})
	return out
}

// referenceOf returns the reference of a reference-style link or
// image, or nil for any other node (inline links included).
func referenceOf(n ast.Node) *ast.ReferenceLink {
	switch t := n.(type) {
	case *ast.Link:
		return t.Reference
	case *ast.Image:
		return t.Reference
	}
	return nil
}

// refUseEdit converts one reference-style link or image node into an
// Edit, or false when the source position can't be recovered (a node
// with no recorded position, or brackets that don't match ref.Type).
func refUseEdit(
	n ast.Node, ref *ast.ReferenceLink, body []byte, lines [][]byte,
	fmOffset int, newName string, bodyIdx bodyLineIndex,
) (Edit, bool) {
	textStart, textEnd := linkTextBounds(n, body)
	labelStart, labelEnd, ok := labelBoundsInBody(body, textStart, textEnd, ref.Type)
	if !ok {
		return Edit{}, false
	}
	startLine := bodyIdx.lineOfOffset(labelStart) + fmOffset
	endLine := bodyIdx.lineOfOffset(labelEnd) + fmOffset
	startCol := labelStart - bodyIdx.lineStart(startLine-fmOffset)
	endCol := labelEnd - bodyIdx.lineStart(endLine-fmOffset)
	startCh := mdtext.UTF16FromByteOffset(lines[startLine-1], startCol)
	endCh := mdtext.UTF16FromByteOffset(lines[endLine-1], endCol)
	return Edit{
		Range: Range{
			Start: Position{Line: startLine - 1, Character: startCh},
			End:   Position{Line: endLine - 1, Character: endCh},
		},
		NewText: newName,
	}, true
}

// labelBoundsInBody returns the body-byte offsets of the label inside
// a reference-style link. For full `[text][label]` the range covers
// the label content; for shortcut `[label]` and collapsed `[label][]`
// the text bracket IS the label. Returns ok=false when the bracket
// structure doesn't match what the reference type implies.
func labelBoundsInBody(body []byte, textStart, textEnd int, refType ast.ReferenceLinkType) (int, int, bool) {
	if textStart < 0 || textEnd < 0 {
		return 0, 0, false
	}
	if refType == ast.ReferenceLinkFull {
		if textEnd >= len(body) || body[textEnd] != ']' {
			return 0, 0, false
		}
		if textEnd+1 >= len(body) || body[textEnd+1] != '[' {
			return 0, 0, false
		}
		labelOpen := textEnd + 2
		for i := labelOpen; i < len(body); i++ {
			if body[i] == '\\' && i+1 < len(body) {
				i++
				continue
			}
			if body[i] == ']' {
				return labelOpen, i, true
			}
		}
		return 0, 0, false
	}
	if textStart <= 0 || body[textStart-1] != '[' {
		return 0, 0, false
	}
	if textEnd >= len(body) || body[textEnd] != ']' {
		return 0, 0, false
	}
	return textStart, textEnd, true
}

// linkTextBounds returns the [start, end) absolute byte offsets of
// the display-text run of a reference-style link or image inside
// body, or (-1, -1) when the node has no recorded source position or
// no closing `]` can be confirmed. The parser records Pos() at the `[`
// (at the `!` for an image), so the bounds hold for any text content:
// emphasis, code spans, nested images, raw HTML, or none (`[][id]`).
//
// A node with a nil Reference is an inline link or image: its close is
// the first `]` past the content that is followed by `(`.
func linkTextBounds(n ast.Node, body []byte) (int, int) {
	open := n.Pos()
	if _, ok := n.(*ast.Image); ok && open >= 0 {
		open++
	}
	if open < 0 || open >= len(body) || body[open] != '[' {
		return -1, -1
	}
	end := closingTextBracket(body, open+1, contentEnd(n, body, open+1), referenceOf(n))
	if end < 0 {
		return -1, -1
	}
	return open + 1, end
}

// contentEnd returns the offset just past the last source byte the
// parser placed inside n — text, raw HTML, an autolink, or a whole
// nested image — or from when n has no such content. The closing `]`
// of the link text sits at or after it, so a `]` inside a nested
// image's destination or reference, a code span, or an HTML attribute
// is never taken for it.
func contentEnd(n ast.Node, body []byte, from int) int {
	end := from
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		stop := -1
		switch t := c.(type) {
		case *ast.Image:
			if c != n {
				if e := imageEnd(t, body); e > end {
					end = e
				}
				return ast.WalkSkipChildren, nil
			}
		case *ast.Text:
			stop = t.Segment.Stop
		case *ast.RawHTML:
			if k := t.Segments.Len(); k > 0 {
				stop = t.Segments.At(k - 1).Stop
			}
		case *ast.AutoLink:
			if t.Pos() >= 0 {
				stop = t.Pos() + len(t.Label(body)) + 2
			}
		}
		if stop > end {
			end = stop
		}
		return ast.WalkContinue, nil
	})
	return end
}

// imageEnd returns the offset just past a nested image's full source
// — `![alt](dest)`, `![alt][label]`, `![alt][]`, or `![alt]` — or -1
// when its text bracket can't be confirmed. contentEnd uses it to keep
// the image's own `][label]` from closing the enclosing link.
func imageEnd(img *ast.Image, body []byte) int {
	_, closeIdx := linkTextBounds(img, body)
	if closeIdx < 0 {
		return -1
	}
	if img.Reference == nil {
		return parenEnd(body, closeIdx+1)
	}
	switch img.Reference.Type {
	case ast.ReferenceLinkFull:
		// linkTextBounds confirmed the `[label]` that follows, so its
		// closing `]` is present.
		return closeIdx + 2 + bytes.IndexByte(body[closeIdx+2:], ']') + 1
	case ast.ReferenceLinkCollapsed:
		return closeIdx + 3
	}
	return closeIdx + 1
}

// parenEnd returns the offset just past the `)` that balances the `(`
// at open, honoring backslash escapes and nested parentheses, or -1
// when there is none.
func parenEnd(body []byte, open int) int {
	depth := 0
	for i := open; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// closingTextBracket returns the offset of the `]` that closes the
// link text starting at textStart, scanning from the content end
// `from`. A candidate `]` must be followed by what ref.Type requires
// (`[label]` for full, `[]` for collapsed), and the label — the bracket
// after it for full, the text itself otherwise — must normalize to
// ref.Value. A nil ref means an inline link or image: the candidate
// must be followed by `(`. Backslash escapes are skipped and a blank
// line ends the search. Returns -1 when no candidate qualifies.
func closingTextBracket(body []byte, textStart, from int, ref *ast.ReferenceLink) int {
	if ref == nil {
		return inlineTextBracket(body, from)
	}
	want := NormalizedLabel(ref.Value)
	for p := from; p < len(body); p++ {
		switch body[p] {
		case '\\':
			p++
			continue
		case '\n':
			if p+1 < len(body) && body[p+1] == '\n' {
				return -1
			}
			continue
		case ']':
		default:
			continue
		}
		var label []byte
		switch ref.Type {
		case ast.ReferenceLinkFull:
			if p+1 >= len(body) || body[p+1] != '[' {
				continue
			}
			q := bytes.IndexByte(body[p+2:], ']')
			if q < 0 {
				return -1
			}
			label = body[p+2 : p+2+q]
		case ast.ReferenceLinkCollapsed:
			if !bytes.HasPrefix(body[p+1:], []byte("[]")) {
				continue
			}
			label = body[textStart:p]
		default:
			label = body[textStart:p]
		}
		if NormalizedLabel(label) == want {
			return p
		}
	}
	return -1
}

// inlineTextBracket returns the offset of the first `]` at or after
// from that is followed by `(` — the close of an inline link's text —
// skipping backslash escapes and stopping at a blank line, or -1.
func inlineTextBracket(body []byte, from int) int {
	for p := from; p < len(body); p++ {
		switch body[p] {
		case '\\':
			p++
		case '\n':
			if p+1 < len(body) && body[p+1] == '\n' {
				return -1
			}
		case ']':
			if p+1 < len(body) && body[p+1] == '(' {
				return p
			}
		}
	}
	return -1
}

// RefDefBracketBytes returns the [start, end) byte offsets of the
// label inside a CommonMark reference-definition line, or nil when
// row is not a reference definition.
func RefDefBracketBytes(row []byte) []int {
	i := 0
	for i < len(row) && i < 3 && row[i] == ' ' {
		i++
	}
	if i >= len(row) || row[i] != '[' {
		return nil
	}
	open := i + 1
	closeIdx := -1
	for j := open; j < len(row); j++ {
		if row[j] == ']' {
			closeIdx = j
			break
		}
	}
	if closeIdx < 0 || closeIdx == open {
		return nil
	}
	if closeIdx+1 >= len(row) || row[closeIdx+1] != ':' {
		return nil
	}
	return []int{open, closeIdx}
}

// lineOfBodyOffset returns the 1-based line of byte offset off within
// body. Linear; tight per-edit loops use bodyLineIndex instead.
func lineOfBodyOffset(body []byte, off int) int {
	if off < 0 {
		return 1
	}
	if off > len(body) {
		off = len(body)
	}
	// bytes.Count special-cases a one-byte separator to a SIMD byte count;
	// a hand-rolled scan loop is not vectorized.
	return 1 + bytes.Count(body[:off], []byte{'\n'})
}

// bodyLineIndex precomputes every line-start offset so a rename
// emitting many per-link edits stays linear instead of quadratic.
type bodyLineIndex struct {
	starts []int
}

func newBodyLineIndex(body []byte) bodyLineIndex {
	starts := make([]int, 1, 1+bodyNewlineCount(body))
	for i, b := range body {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return bodyLineIndex{starts: starts}
}

func bodyNewlineCount(body []byte) int {
	n := 0
	for _, b := range body {
		if b == '\n' {
			n++
		}
	}
	return n
}

func (b bodyLineIndex) lineOfOffset(off int) int {
	if off < 0 {
		return 1
	}
	lo, hi := 0, len(b.starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if b.starts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1
}

func (b bodyLineIndex) lineStart(n int) int {
	if n < 1 || n > len(b.starts) {
		return -1
	}
	return b.starts[n-1]
}

// bodyAndFMOffset splits source into body and the line offset the
// front matter contributed, mirroring the index's slicing so renames
// work consistently with or without front matter.
func bodyAndFMOffset(source []byte) ([]byte, int) {
	fm, body := lint.StripFrontMatter(source)
	off := 0
	for _, b := range fm {
		if b == '\n' {
			off++
		}
	}
	return body, off
}

// splitLines splits source into lines, dropping a trailing `\r` so
// CRLF and LF files yield the same per-line byte ranges. It mirrors
// the LSP server's splitLines exactly (including the empty-input
// one-element contract) so a rename's edit coordinates are
// byte-identical across the two surfaces.
func splitLines(source []byte) [][]byte {
	if len(source) == 0 {
		return [][]byte{nil}
	}
	parts := bytes.Split(source, []byte{'\n'})
	for i, p := range parts {
		if n := len(p); n > 0 && p[n-1] == '\r' {
			parts[i] = p[:n-1]
		}
	}
	return parts
}
