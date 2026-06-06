// Package rename is the workspace rename engine shared by the LSP
// server and the `mdsmith rename` CLI. It answers one question:
// given a file's source and a rename target (a heading or a
// link-reference label), what edits perform the rename, or what
// typed conflict prevents it? It speaks no LSP wire types — callers
// adapt the neutral Edit/Position/Range values to their surface.
package rename

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
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
	return fmt.Sprintf("label cannot contain %q", e.Rune)
}

// LabelConflictError reports that the new label collides with another
// reference definition already in the file. Conflict carries the
// colliding label's original (non-normalized) spelling.
type LabelConflictError struct{ Conflict string }

func (e LabelConflictError) Error() string {
	return "rename would collide with link reference [" + e.Conflict + "]"
}

// LinkRef computes the in-file edits that rename a link-reference
// label from oldLabel (already normalized via util.ToLinkReference)
// to newName. The rewrite is file-local: the `[label]: url`
// definition plus every `[text][label]` and shortcut `[label]` use.
//
// Returns ErrEmptyLabel, an InvalidLabelRuneError, or a
// LabelConflictError without producing any edit when the rename is
// unsafe, so callers can surface the failure before applying.
func LinkRef(source []byte, oldLabel, newName string) ([]Edit, error) {
	if strings.TrimSpace(newName) == "" {
		return nil, ErrEmptyLabel
	}
	if invalid := invalidLinkRefRune(newName); invalid != 0 {
		return nil, InvalidLabelRuneError{Rune: invalid}
	}
	// A rename that keeps the same normalized label (e.g. "docs api"
	// → "Docs API") is allowed — it refreshes casing/spacing across
	// the def and every use. labelConflict matches on the normalized
	// form so such a rename never collides with itself.
	newLabel := normalizedLabel([]byte(newName))
	if conflict := labelConflict(source, oldLabel, newLabel); conflict != "" {
		return nil, LabelConflictError{Conflict: conflict}
	}
	return linkRefEdits(source, oldLabel, newName), nil
}

// ValidRefDefBodyLines reports the body-line indices that hold a real
// reference definition goldmark accepted (not a code-block
// look-alike). The LSP prepare-rename gate consults it so the rename
// UI never surfaces on a `[label]: url`-shaped code sample.
func ValidRefDefBodyLines(body []byte) map[int]bool {
	out := map[int]bool{}
	for _, m := range validRefDefMatches(body) {
		out[m.bodyLine] = true
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
// are rejected outright rather than auto-escaped.
func invalidLinkRefRune(s string) rune {
	for _, r := range s {
		switch r {
		case '\n', '\r', '[', ']':
			return r
		}
	}
	return 0
}

func normalizedLabel(b []byte) string {
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
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	consumed := contentBlockLines(root, body)
	var out []validRefDefMatch
	for _, m := range index.RefDefRegexpMatches(body) {
		bodyLine := lineOfBodyOffset(body, m[2])
		if consumed[bodyLine] {
			continue
		}
		raw := body[m[2]:m[3]]
		norm := normalizedLabel(raw)
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
func contentBlockLines(root ast.Node, body []byte) map[int]bool {
	out := map[int]bool{}
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
			out[lineOfBodyOffset(body, seg.Start)] = true
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
		bracket := refDefBracketBytes(row)
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

// refUseEditsInBody walks the AST for ast.Link nodes whose Reference
// matches oldLabel and emits one Edit per use.
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
		l, ok := n.(*ast.Link)
		if !ok || l.Reference == nil {
			return ast.WalkContinue, nil
		}
		if normalizedLabel(l.Reference.Value) != oldLabel {
			return ast.WalkContinue, nil
		}
		edit, ok := refUseEdit(l, body, lines, fmOffset, newName, idx)
		if ok {
			out = append(out, edit)
		}
		return ast.WalkContinue, nil
	})
	return out
}

// refUseEdit converts one link node into an Edit, or false when the
// source position can't be recovered (e.g. an empty-text reference
// like `[][id]` that linkTextBounds can't anchor).
func refUseEdit(
	l *ast.Link, body []byte, lines [][]byte, fmOffset int, newName string,
	bodyIdx bodyLineIndex,
) (Edit, bool) {
	textStart, textEnd := linkTextBounds(l, body)
	labelStart, labelEnd, ok := labelBoundsInBody(body, textStart, textEnd, l.Reference.Type)
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
// the link's display-text run inside body, or (-1, -1) when the link
// has no parsed text segment.
func linkTextBounds(l *ast.Link, body []byte) (int, int) {
	start, end := -1, -1
	_ = ast.Walk(l, func(cur ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		t, ok := cur.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		if start < 0 || t.Segment.Start < start {
			start = t.Segment.Start
		}
		if t.Segment.Stop > end {
			end = t.Segment.Stop
		}
		return ast.WalkContinue, nil
	})
	return start, end
}

// refDefBracketBytes returns the [start, end) byte offsets of the
// label inside a CommonMark reference-definition line, or nil when
// row is not a reference definition.
func refDefBracketBytes(row []byte) []int {
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
	line := 1
	for i := 0; i < off; i++ {
		if body[i] == '\n' {
			line++
		}
	}
	return line
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
