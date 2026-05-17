package rename

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Workspace is the seam the heading rename needs. A heading rename
// reaches beyond the edited file: every incoming `[t](other.md#slug)`
// anchor and every `[label]: other.md#slug` ref-def whose destination
// resolves to the renamed heading must be rewritten too. The engine
// asks the Workspace three questions and stays surface-neutral:
//
//   - which edges point at (file, slug)?
//   - what files exist?
//   - what key + bytes back a workspace-relative path?
//
// The LSP server backs it with its warm index plus open buffers; the
// `mdsmith rename` CLI with a transient index plus disk reads. The
// returned key groups edits per output target (an LSP document URI,
// or a CLI file path) without the engine knowing which.
type Workspace interface {
	// IncomingAnchorEdges returns every workspace edge whose target
	// is (file, slug). file is workspace-relative.
	IncomingAnchorEdges(file, slug string) []index.Edge
	// Files lists every workspace-relative file path the workspace
	// knows about.
	Files() []string
	// Resolve maps a workspace-relative path to the opaque key its
	// edits group under and the file's current bytes (open-buffer
	// text when the surface has one, else disk). ok is false when
	// the file is unreadable.
	Resolve(file string) (key string, source []byte, ok bool)
}

// ErrEmptyHeadingSlug is returned when the new heading text slugifies
// to nothing (punctuation-only): the renamed heading would have no
// addressable anchor, so cross-file links would have nowhere to land.
var ErrEmptyHeadingSlug = errors.New(
	"new heading text has no addressable slug; pick text containing letters or digits")

// InvalidHeadingRuneError reports the first newline / carriage return
// in the new heading text. A control rune would split the single-line
// heading into a multi-line insertion. Its Error() is byte-identical
// to the message the LSP server emitted before the refactor so the
// adapter can surface it verbatim.
type InvalidHeadingRuneError struct{ Rune rune }

func (e InvalidHeadingRuneError) Error() string {
	return fmt.Sprintf("heading text cannot contain %q", e.Rune)
}

// HeadingCollisionError reports that the new heading's bare slug
// collides with another heading in the same file. Conflict carries
// the colliding heading's visible text. Its Error() matches the
// pre-refactor LSP message verbatim.
type HeadingCollisionError struct{ Conflict string }

func (e HeadingCollisionError) Error() string {
	return "rename would collide with heading " + e.Conflict
}

// Heading rewrites a heading and every workspace anchor link / ref-def
// destination pointing at it. fileKey is the key the heading file's
// own edits group under (the LSP document URI, or the CLI file path);
// file is its workspace-relative path; source its current bytes; line
// the 1-based source line of the heading; oldName the heading's
// current visible text; newName the requested text.
//
// Returns an empty (non-nil) map with no error when the rename is a
// no-op (newName equals oldName after trimming). Returns a typed
// error — InvalidHeadingRuneError, ErrEmptyHeadingSlug, or
// HeadingCollisionError — without producing any edit when the rename
// is unsafe, so callers surface the failure before applying anything.
//
// Algorithm (unchanged from the LSP original):
//  1. Recompute the file's heading slug map under the new text.
//  2. Reject a new bare-slug collision with another heading.
//  3. Diff old vs new slug maps to find shifted headings.
//  4. Emit the heading-line edit plus, per shifted slug, every
//     incoming anchor edit and every ref-def-destination edit.
func Heading(
	ws Workspace, fileKey, file string, source []byte,
	line int, oldName, newName string,
) (map[string][]Edit, error) {
	if strings.TrimSpace(newName) == strings.TrimSpace(oldName) {
		return map[string][]Edit{}, nil
	}
	if r := firstControlRune(newName); r != 0 {
		return nil, InvalidHeadingRuneError{Rune: r}
	}
	if mdtext.Slugify(newName) == "" {
		return nil, ErrEmptyHeadingSlug
	}
	oldSlugs, newSlugs, conflict := computeSlugRemap(source, line, newName)
	if conflict != "" {
		return nil, HeadingCollisionError{Conflict: conflict}
	}
	// headingTextEdit's false branch is unreachable here: the caller
	// resolved `line` to a heading line, so the row is a heading.
	headingEdit, _ := headingTextEdit(source, line, newName)
	changes := map[string][]Edit{fileKey: {headingEdit}}
	for old, neu := range slugRemapPairs(oldSlugs, newSlugs) {
		appendAnchorEditsForHeading(changes, ws, file, old, neu)
		appendRefDefDestEditsForHeading(changes, ws, file, old, neu)
	}
	stableSortEdits(changes)
	return changes, nil
}

// FindHeadingLine returns the 1-based source line of the first
// heading whose visible text equals headingText, or ok=false when no
// heading matches. The CLI uses it to turn `--heading "Old"` into the
// line coordinate the rename engine expects; it parses the same way
// the engine does so the line it finds is the line Heading rewrites.
func FindHeadingLine(source []byte, headingText string) (int, bool) {
	body, fmOffset := bodyAndFMOffset(source)
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	for _, h := range walkAllHeadings(root, body) {
		if h.text == headingText {
			return h.bodyLine + fmOffset, true
		}
	}
	return 0, false
}

// NormalizeLabel collapses a link-reference label to its canonical
// matching form (lowercased, whitespace-collapsed), the same
// normalization LinkRef expects its oldLabel argument in. The CLI
// normalizes `--link-ref oldlabel` through this before calling
// LinkRef.
func NormalizeLabel(s string) string {
	return normalizedLabel([]byte(s))
}

// firstControlRune returns the first newline / carriage return in s,
// or 0 when s is a single line. Heading text and link-ref labels are
// single-line surfaces; a control rune would rewrite them into
// multi-line garbage.
func firstControlRune(s string) rune {
	for _, r := range s {
		if r == '\n' || r == '\r' {
			return r
		}
	}
	return 0
}

// computeSlugRemap returns the file's old slug list, its new slug list
// (after substituting newText for the heading on `line`), and a
// non-empty `conflict` heading text when the rename would create a new
// duplicate base slug. The slices share index ordering: oldSlugs[i] /
// newSlugs[i] are the i-th heading's slug before / after.
//
// The walk includes every heading, even ones whose slug is empty
// (punctuation-only). Skipping them — as mdtext.CollectTOCItems does —
// would desynchronize the per-heading indices from the heading-line
// walk and mis-identify the renamed heading on files with empty-slug
// headings before the cursor's line. It would also block renaming an
// empty-slug heading into a real one (which can shift disambiguators).
func computeSlugRemap(source []byte, line int, newText string) ([]string, []string, string) {
	body, fmOffset := bodyAndFMOffset(source)
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	headings := walkAllHeadings(root, body)
	bodyLine := line - fmOffset
	target := -1
	for i, h := range headings {
		if h.bodyLine == bodyLine {
			target = i
			break
		}
	}
	if target < 0 {
		return nil, nil, ""
	}
	texts := make([]string, len(headings))
	for i, h := range headings {
		texts[i] = h.text
	}
	oldBase := mdtext.Slugify(headings[target].text)
	texts[target] = newText
	// Collision check: any other heading shares the new bare slug?
	// Skip it when the rename keeps the same base slug — a
	// non-semantic edit (case, punctuation) inside an existing
	// duplicate-name group doesn't introduce a new collision, so
	// blocking it would surprise the user.
	newBase := mdtext.Slugify(newText)
	if newBase != "" && newBase != oldBase {
		for i, t := range texts {
			if i == target {
				continue
			}
			if mdtext.Slugify(t) == newBase {
				return nil, nil, headings[i].text
			}
		}
	}
	oldSlugs := assignSlugs(slicesOfText(headings))
	newSlugs := assignSlugs(texts)
	return oldSlugs, newSlugs, ""
}

// headingWalk records the body-line and visible text of one heading
// node. It carries no slug — the slug is computed in lockstep with
// the rename so empty-slug headings aren't silently dropped.
type headingWalk struct {
	bodyLine int
	text     string
}

// walkAllHeadings returns every heading in document order, including
// ones whose slugified text is empty.
func walkAllHeadings(root ast.Node, body []byte) []headingWalk {
	var out []headingWalk
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		out = append(out, headingWalk{
			bodyLine: lineOfBodyOffset(body, h.Lines().At(0).Start),
			text:     mdtext.ExtractPlainText(h, body),
		})
		return ast.WalkContinue, nil
	})
	return out
}

func slicesOfText(walks []headingWalk) []string {
	out := make([]string, len(walks))
	for i, w := range walks {
		out[i] = w.text
	}
	return out
}

// assignSlugs runs the same disambiguator pass
// mdtext.CollectTOCItems uses, but over a parallel slice of texts so
// callers can substitute a renamed heading's text in place without
// losing alignment with the heading walk. Empty-base-slug headings
// stay at "" — those have no anchor and never participate in link
// rewrites.
func assignSlugs(texts []string) []string {
	used := map[string]bool{}
	counts := map[string]int{}
	out := make([]string, len(texts))
	for i, t := range texts {
		base := mdtext.Slugify(t)
		if base == "" {
			out[i] = ""
			continue
		}
		anchor := base
		if used[anchor] {
			c := counts[base]
			for {
				c++
				anchor = fmt.Sprintf("%s-%d", base, c)
				if !used[anchor] {
					break
				}
			}
			counts[base] = c
		}
		used[anchor] = true
		out[i] = anchor
	}
	return out
}

// slugRemapPairs maps old slug → new slug for every heading whose slug
// changed. It skips entries whose old slug is empty (no anchor).
func slugRemapPairs(oldSlugs, newSlugs []string) map[string]string {
	out := map[string]string{}
	for i := range oldSlugs {
		if oldSlugs[i] == "" || oldSlugs[i] == newSlugs[i] {
			continue
		}
		// First wins when two old slugs map to one new slug — the
		// disambiguator keeps slugs unique so this shouldn't happen,
		// but the guard prevents an inflated rewrite from a corrupt
		// index.
		if _, exists := out[oldSlugs[i]]; !exists {
			out[oldSlugs[i]] = newSlugs[i]
		}
	}
	return out
}

// headingTextEdit replaces the heading text on the source line with
// newName. Returns false when the line is not a recognized heading
// line.
func headingTextEdit(source []byte, line int, newName string) (Edit, bool) {
	lines := splitLines(source)
	if line-1 >= len(lines) {
		return Edit{}, false
	}
	row := lines[line-1]
	startByte, endByte, ok := atxHeadingTextByteRange(row)
	if !ok {
		startByte, endByte = trimmedRange(row)
	}
	startCh := utf16FromByteOffset(row, startByte)
	endCh := utf16FromByteOffset(row, endByte)
	return Edit{
		Range: Range{
			Start: Position{Line: line - 1, Character: startCh},
			End:   Position{Line: line - 1, Character: endCh},
		},
		NewText: newName,
	}, true
}

// atxHeadingTextByteRange returns the byte offsets of the heading text
// inside an ATX heading line — the run between the opening `#`s (and
// required following space) and any trailing closing `#` run. Returns
// false when row is not an ATX heading line.
//
// Trailing markers are recognized only when a CommonMark-significant
// space precedes the run, mirroring goldmark's ATX parsing. A heading
// line with no text (`### `) returns a zero-width range where text
// would begin so the editor inserts there rather than rejecting the
// rename.
func atxHeadingTextByteRange(row []byte) (int, int, bool) {
	textStart, ok := atxHeadingTextStart(row)
	if !ok {
		return 0, 0, false
	}
	end := trimRightSpace(row, textStart, len(row))
	end = trimTrailingHashRun(row, textStart, end)
	return textStart, end, true
}

// atxHeadingTextStart returns the byte offset where a heading's text
// run begins, or false when row is not an ATX heading line.
func atxHeadingTextStart(row []byte) (int, bool) {
	i := skipLeadingSpaces(row, 3)
	if i >= len(row) || row[i] != '#' {
		return 0, false
	}
	hashStart := i
	for i < len(row) && row[i] == '#' {
		i++
	}
	level := i - hashStart
	if level < 1 || level > 6 {
		return 0, false
	}
	// CommonMark requires a space (or end of line) after the markers.
	// `##foo` is paragraph content even though it starts with `#`.
	if i < len(row) && row[i] != ' ' && row[i] != '\t' {
		return 0, false
	}
	for i < len(row) && (row[i] == ' ' || row[i] == '\t') {
		i++
	}
	return i, true
}

// trimTrailingHashRun strips a trailing `#` run preceded by whitespace
// — the optional ATX closing markers. A `#` run with no preceding
// whitespace is part of the heading text (e.g. `# foo#bar`).
func trimTrailingHashRun(row []byte, start, end int) int {
	if end <= start || row[end-1] != '#' {
		return end
	}
	k := end
	for k > start && row[k-1] == '#' {
		k--
	}
	if k <= start || (row[k-1] != ' ' && row[k-1] != '\t') {
		return end
	}
	return trimRightSpace(row, start, k-1)
}

// skipLeadingSpaces advances past up to max leading space bytes.
func skipLeadingSpaces(row []byte, max int) int {
	i := 0
	for i < len(row) && i < max && row[i] == ' ' {
		i++
	}
	return i
}

// trimRightSpace returns end shrunk past trailing space/tab bytes in
// row[start:end].
func trimRightSpace(row []byte, start, end int) int {
	for end > start && (row[end-1] == ' ' || row[end-1] == '\t') {
		end--
	}
	return end
}

// trimmedRange returns the byte offsets of row stripped of leading and
// trailing horizontal whitespace.
func trimmedRange(row []byte) (int, int) {
	start, end := 0, len(row)
	for start < end && (row[start] == ' ' || row[start] == '\t') {
		start++
	}
	for end > start && (row[end-1] == ' ' || row[end-1] == '\t') {
		end--
	}
	return start, end
}

// appendAnchorEditsForHeading records one Edit per workspace anchor
// link pointing at (headingFile, oldSlug). The new slug is
// substituted into each link destination's fragment; the path
// component is left unchanged so relative includes keep their source
// form.
func appendAnchorEditsForHeading(
	changes map[string][]Edit, ws Workspace,
	headingFile, oldSlug, newSlug string,
) {
	for _, e := range ws.IncomingAnchorEdges(headingFile, oldSlug) {
		key, edit, ok := anchorEditForEdge(ws, e, oldSlug, newSlug)
		if !ok {
			continue
		}
		changes[key] = append(changes[key], edit)
	}
}

// anchorEditForEdge converts one incoming-edge record into a concrete
// Edit. Returns false when the source file isn't readable (out of
// workspace, deleted) or the edge's link can't be located — the
// rename skips those rather than failing the whole request, since the
// alternative would block a heading rename over an unrelated stale
// edge.
func anchorEditForEdge(ws Workspace, e index.Edge, oldSlug, newSlug string) (string, Edit, bool) {
	key, source, ok := ws.Resolve(e.SourceFile)
	if !ok {
		return "", Edit{}, false
	}
	lines := splitLines(source)
	// The index can hold stale entries after a closed-buffer edit or
	// an unprocessed watcher event. Indexing past EOF would panic.
	if e.SourceLine < 1 || e.SourceLine > len(lines) {
		return "", Edit{}, false
	}
	row := lines[e.SourceLine-1]
	startByte, endByte, ok := anchorFragmentBytes(row, e.SourceCol-1, oldSlug)
	if !ok {
		return "", Edit{}, false
	}
	startCh := utf16FromByteOffset(row, startByte)
	endCh := utf16FromByteOffset(row, endByte)
	return key, Edit{
		Range: Range{
			Start: Position{Line: e.SourceLine - 1, Character: startCh},
			End:   Position{Line: e.SourceLine - 1, Character: endCh},
		},
		NewText: newSlug,
	}, true
}

// anchorFragmentBytes locates the byte range of the fragment slug
// inside a link destination on row, starting from the link's
// text-start column. Returns the raw fragment range (excluding the
// leading `#`).
//
// The match is normalized: the raw fragment is URL-unescaped and run
// through mdtext.Slugify, mirroring how the index keys incoming
// edges, so `(#Setup)` and `(#Docs%20API)` both participate in a
// rename even though their literal bytes differ from `setup` /
// `docs-api`.
func anchorFragmentBytes(row []byte, textStart int, oldSlug string) (int, int, bool) {
	bracketStart := textStart
	if bracketStart < 0 {
		bracketStart = 0
	}
	if bracketStart >= len(row) {
		return 0, 0, false
	}
	open, closeIdx, ok := destBounds(row, bracketStart)
	if !ok {
		return 0, 0, false
	}
	hash := indexOfHash(row, open, closeIdx)
	if hash < 0 {
		return 0, 0, false
	}
	fragEnd := fragmentEnd(row, hash+1, closeIdx)
	rawFrag := row[hash+1 : fragEnd]
	if !fragmentMatchesSlug(rawFrag, oldSlug) {
		return 0, 0, false
	}
	return hash + 1, fragEnd, true
}

// destBounds returns the byte offsets of the destination content —
// the byte just after `(` and the byte at the matching `)` — for a
// link starting at or after `from`. Nested parens are matched
// depth-aware and backslash-escaped parens are literal, so
// `[t](foo\(bar\)#sec)` resolves to the outer parens.
func destBounds(row []byte, from int) (int, int, bool) {
	open := -1
	for i := from; i < len(row)-1; i++ {
		// Skip backslash-escaped bytes so a literal `\]` in the text
		// portion doesn't fool the `](` lookahead.
		if row[i] == '\\' && i+1 < len(row) {
			i++
			continue
		}
		if row[i] == ']' && row[i+1] == '(' {
			open = i + 2
			break
		}
	}
	if open < 0 {
		return 0, 0, false
	}
	closeIdx := -1
	depth := 1
	for j := open; j < len(row) && closeIdx < 0; j++ {
		if isBackslashEscaped(row, j) {
			continue
		}
		switch row[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closeIdx = j
			}
		}
	}
	if closeIdx < 0 {
		return 0, 0, false
	}
	return open, closeIdx, true
}

// isBackslashEscaped reports whether row[i] is preceded by an odd
// number of backslashes — the CommonMark escape signal.
func isBackslashEscaped(row []byte, i int) bool {
	n := 0
	for k := i - 1; k >= 0 && row[k] == '\\'; k-- {
		n++
	}
	return n%2 == 1
}

// indexOfHash returns the offset of the first `#` in row[open:close],
// or -1 when the destination has no fragment.
func indexOfHash(row []byte, open, closeIdx int) int {
	for i := open; i < closeIdx; i++ {
		if row[i] == '#' {
			return i
		}
	}
	return -1
}

// fragmentEnd returns the byte offset where a fragment ends within
// row[start:close]. Inline-link destinations carry no query strings,
// so the fragment runs to the first whitespace, the first `>`
// (angle-bracketed `<url>` form), or to close. Without the `>` guard,
// `[t](<#sec>)` would slugify `sec>` and overwrite the closing
// bracket.
func fragmentEnd(row []byte, start, closeIdx int) int {
	for i := start; i < closeIdx; i++ {
		if row[i] == ' ' || row[i] == '\t' || row[i] == '>' {
			return i
		}
	}
	return closeIdx
}

// fragmentMatchesSlug reports whether the raw fragment bytes slugify
// to oldSlug. URL-unescape mirrors the index's decodeAnchor so `%20`
// decodes the same way before the slug pass.
func fragmentMatchesSlug(rawFrag []byte, oldSlug string) bool {
	decoded, err := url.PathUnescape(string(rawFrag))
	if err != nil {
		decoded = string(rawFrag)
	}
	return mdtext.Slugify(decoded) == oldSlug
}

// appendRefDefDestEditsForHeading rewrites `[label]: url` definitions
// whose destination points at the renamed heading. The index treats
// ref-defs as symbols, not edges, so the anchor-edge pass skips them;
// without this companion pass a heading rename would strand def lines
// like `[setup]: ./a.md#setup` on the old slug while every
// `[t][setup]` use still resolved through the def to a stale anchor.
//
// It walks every workspace file, reads each through ws.Resolve so
// unsaved buffers win over disk, and emits one edit per def whose
// destination fragment slugifies to oldSlug AND whose path resolves
// to headingFile (or is the same file when the def is anchor-only).
func appendRefDefDestEditsForHeading(
	changes map[string][]Edit, ws Workspace,
	headingFile, oldSlug, newSlug string,
) {
	headingFile = index.NormalizePath(headingFile)
	for _, rel := range ws.Files() {
		key, source, ok := ws.Resolve(rel)
		if !ok {
			continue
		}
		body, fmOffset := bodyAndFMOffset(source)
		fileLines := splitLines(source)
		// Route through validRefDefMatches so `[label]:` lines
		// goldmark refused (code blocks, paragraph continuations)
		// aren't rewritten as if they were real defs.
		for _, m := range validRefDefMatches(body) {
			edit, ok := refDefDestEditForMatch(
				body, fileLines, fmOffset, m.matchIdx,
				rel, headingFile, oldSlug, newSlug,
			)
			if !ok {
				continue
			}
			changes[key] = append(changes[key], edit)
		}
	}
}

// refDefDestEditForMatch turns one regex match for a `[label]: url`
// line into an Edit on the URL's slug portion, or ok=false when the
// destination doesn't point at the renamed heading.
func refDefDestEditForMatch(
	body []byte, fileLines [][]byte, fmOffset int, m []int,
	defFile, headingFile, oldSlug, newSlug string,
) (Edit, bool) {
	bodyLine := lineOfBodyOffset(body, m[2])
	fileLine := bodyLine + fmOffset
	if fileLine-1 >= len(fileLines) {
		return Edit{}, false
	}
	row := fileLines[fileLine-1]
	colonOff := refDefColonOffset(row)
	if colonOff < 0 {
		return Edit{}, false
	}
	destStart, destEnd := refDefDestRange(row, colonOff+1)
	if destStart >= destEnd {
		return Edit{}, false
	}
	dest := row[destStart:destEnd]
	if !refDefDestPointsAt(dest, defFile, headingFile, oldSlug) {
		return Edit{}, false
	}
	// refDefDestPointsAt already confirmed dest contains `#oldSlug`
	// (oldSlug is non-empty on this path), so `#` always exists.
	hashIdx := destStart
	for hashIdx < destEnd && row[hashIdx] != '#' {
		hashIdx++
	}
	fragEnd := fragmentEnd(row, hashIdx+1, destEnd)
	startCh := utf16FromByteOffset(row, hashIdx+1)
	endCh := utf16FromByteOffset(row, fragEnd)
	return Edit{
		Range: Range{
			Start: Position{Line: fileLine - 1, Character: startCh},
			End:   Position{Line: fileLine - 1, Character: endCh},
		},
		NewText: newSlug,
	}, true
}

// refDefColonOffset returns the byte offset of the `:` that follows
// `[label]` in a ref-def line, or -1 when the shape doesn't match:
// ≤3 leading spaces, then `[label]:` where label has no `]`.
func refDefColonOffset(row []byte) int {
	i := 0
	for i < len(row) && i < 3 && row[i] == ' ' {
		i++
	}
	if i >= len(row) || row[i] != '[' {
		return -1
	}
	closeIdx := -1
	for j := i + 1; j < len(row); j++ {
		if row[j] == ']' {
			closeIdx = j
			break
		}
	}
	if closeIdx < 0 || closeIdx+1 >= len(row) || row[closeIdx+1] != ':' {
		return -1
	}
	return closeIdx + 1
}

// refDefDestRange returns the byte range of the destination URL
// portion of a ref-def line, given the offset just past the `:`.
// Skips leading whitespace, then handles both forms:
//
//   - Angle-bracketed `[label]: <url>`: the range covers the bytes
//     inside the angle brackets, so a slug edit leaves `<` / `>`
//     intact.
//   - Bare `[label]: url`: from the first non-whitespace byte to the
//     next whitespace.
//
// Quoted titles after the URL are excluded either way.
func refDefDestRange(row []byte, from int) (int, int) {
	i := from
	for i < len(row) && (row[i] == ' ' || row[i] == '\t') {
		i++
	}
	if i < len(row) && row[i] == '<' {
		open := i + 1
		for j := open; j < len(row); j++ {
			if row[j] == '\\' && j+1 < len(row) {
				j++
				continue
			}
			if row[j] == '>' {
				return open, j
			}
		}
		// Unterminated `<…` — fall through to the bare reader from
		// the original `<` position so a malformed line doesn't
		// masquerade as an angle-bracketed dest.
	}
	start := i
	for i < len(row) && row[i] != ' ' && row[i] != '\t' {
		i++
	}
	return start, i
}

// refDefDestPointsAt reports whether dest (the URL portion of a
// ref-def in defFile) resolves to (headingFile, oldSlug). It mirrors
// the index's edge collector: percent-decode the fragment, slugify,
// compare to oldSlug; resolve the path against defFile's directory
// and compare to headingFile.
func refDefDestPointsAt(dest []byte, defFile, headingFile, oldSlug string) bool {
	t, ok := refDefParseTarget(string(dest))
	if !ok {
		return false
	}
	// url.Parse already decoded t.fragment, but re-running
	// PathUnescape mirrors the index's decodeAnchor so corner-case
	// escapes can't drift. The error is impossible here: url.Parse
	// would have rejected a malformed `%xx`.
	decoded, _ := url.PathUnescape(t.fragment)
	if mdtext.Slugify(decoded) != oldSlug {
		return false
	}
	if t.localAnchor {
		return index.NormalizePath(defFile) == headingFile
	}
	resolved := linkgraph.ResolveRelTarget(defFile, t.path)
	return resolved == headingFile
}

// refDefDestTarget is the parsed shape of a ref-def URL. Kept private
// — the index has its own linkTarget helper and leaking two slightly
// different parsers through one type would invite drift.
type refDefDestTarget struct {
	path        string
	fragment    string
	localAnchor bool
}

func refDefParseTarget(dest string) (refDefDestTarget, bool) {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.HasPrefix(dest, "//") {
		return refDefDestTarget{}, false
	}
	u, err := url.Parse(dest)
	if err != nil {
		return refDefDestTarget{}, false
	}
	if u.Scheme != "" || u.Host != "" {
		return refDefDestTarget{}, false
	}
	if u.Path == "" && u.Fragment != "" {
		return refDefDestTarget{fragment: u.Fragment, localAnchor: true}, true
	}
	if u.Path == "" {
		return refDefDestTarget{}, false
	}
	return refDefDestTarget{path: u.Path, fragment: u.Fragment}, true
}

// stableSortEdits sorts each key's Edit slice in reverse document
// order so a consumer applying edits sequentially ends up with the
// right buffer state: earlier (later-positioned) edits don't shift
// the offsets the next edit relies on, particularly when two edits
// share a line.
func stableSortEdits(changes map[string][]Edit) {
	for key, edits := range changes {
		sort.SliceStable(edits, func(i, j int) bool {
			a, b := edits[i].Range.Start, edits[j].Range.Start
			if a.Line != b.Line {
				return a.Line > b.Line
			}
			return a.Character > b.Character
		})
		changes[key] = edits
	}
}
