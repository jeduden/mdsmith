package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/lsp/index"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// handlePrepareRename answers textDocument/prepareRename. The
// returned range is what an editor highlights in the rename popup;
// for a heading that excludes the leading `#` markers and any
// trailing closing markers so the user types just the heading text,
// not the raw line. Returning null short-circuits the rename so the
// editor never opens the popup at unsupported positions.
func (s *Server) handlePrepareRename(msg *requestMessage) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid prepareRename params")
		return
	}
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, nil)
		return
	}
	res, ok := s.prepareRenameAt(source, rel, p.Position)
	if !ok {
		_ = s.t.writeResponse(msg.ID, nil)
		return
	}
	_ = s.t.writeResponse(msg.ID, res)
}

// prepareRenameAt resolves the source position to a renameable
// symbol (heading text, link-ref label, or shortcut label use) and
// returns the {range, placeholder} payload.
func (s *Server) prepareRenameAt(source []byte, rel string, pos Position) (prepareRenameResult, bool) {
	line := pos.Line + 1
	col := lspPositionToByteColumn(source, line, pos.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)
	switch res.Tag {
	case index.TokenHeading:
		return headingPrepareRange(source, line, res.Name)
	case index.TokenRefDef:
		return refDefPrepareRange(source, line, res.Label)
	case index.TokenRefUse:
		return refUsePrepareRange(source, line, col, res.Label)
	case index.TokenAnchorLink:
		// The cursor sits inside `[text](#anchor)`. Renaming the
		// anchor here would mean "rename the heading this points
		// at"; that semantics belongs on the heading itself, where
		// the WorkspaceEdit also covers the heading-line text.
		// Returning null tells the client there's no rename here.
		return prepareRenameResult{}, false
	}
	return prepareRenameResult{}, false
}

// headingPrepareRange builds the rename range for an ATX or setext
// heading line. ATX headings have their leading and trailing `#`
// markers excluded; setext headings cover the full text line. The
// underline of a setext heading is left alone — CommonMark does not
// require its width to match the text, so the rename never touches
// it.
func headingPrepareRange(source []byte, line int, name string) (prepareRenameResult, bool) {
	lines := splitLines(source)
	if line-1 >= len(lines) {
		return prepareRenameResult{}, false
	}
	row := lines[line-1]
	startCol, endCol, ok := atxHeadingTextByteRange(row)
	if !ok {
		// Not an ATX heading — must be the text line of a setext
		// heading. Cover the full text line, excluding leading and
		// trailing whitespace so the rename doesn't pad the new
		// text against indented setext underlines.
		startCol, endCol = trimmedRange(row)
	}
	startCh := utf16FromByteOffset(row, startCol)
	endCh := utf16FromByteOffset(row, endCol)
	return prepareRenameResult{
		Range: Range{
			Start: Position{Line: line - 1, Character: startCh},
			End:   Position{Line: line - 1, Character: endCh},
		},
		Placeholder: name,
	}, true
}

// atxHeadingTextByteRange returns the byte offsets of the heading
// text inside an ATX heading line — the run between the opening
// `#`s (and required following space) and any trailing closing `#`
// run. Returns false when row is not an ATX heading line.
//
// Trailing markers are recognized only when a CommonMark-significant
// space precedes the run, mirroring goldmark's own ATX parsing
// behavior. A heading line with no text at all (`### `) returns a
// zero-width range at the spot where text would begin so the editor
// inserts there rather than rejecting the rename.
func atxHeadingTextByteRange(row []byte) (int, int, bool) {
	textStart, ok := atxHeadingTextStart(row)
	if !ok {
		return 0, 0, false
	}
	end := trimRightSpace(row, textStart, len(row))
	end = trimTrailingHashRun(row, textStart, end)
	if end < textStart {
		end = textStart
	}
	return textStart, end, true
}

// atxHeadingTextStart returns the byte offset where a heading's
// text run begins, or false when row is not an ATX heading line.
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

// trimTrailingHashRun strips a trailing `#` run that's preceded by
// whitespace — the optional ATX closing markers. A `#` run with no
// preceding whitespace is part of the heading text (e.g. `# foo#bar`).
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

// skipLeadingSpaces advances past up to `max` leading space bytes in
// row and returns the resulting offset.
func skipLeadingSpaces(row []byte, max int) int {
	i := 0
	for i < len(row) && i < max && row[i] == ' ' {
		i++
	}
	return i
}

// trimRightSpace returns end shrunk past any trailing space/tab
// bytes in row[start:end].
func trimRightSpace(row []byte, start, end int) int {
	for end > start && (row[end-1] == ' ' || row[end-1] == '\t') {
		end--
	}
	return end
}

// trimmedRange returns the byte offsets of row stripped of leading
// and trailing horizontal whitespace.
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

// refDefPrepareRange builds the rename range for a `[label]: url`
// definition. The range covers the label between `[` and `]`. The
// placeholder is the raw source slice — Locator's `label` field is
// normalized via util.ToLinkReference (lowercased + whitespace
// collapsed), which would mismatch the document's actual casing
// when the editor pre-fills the rename popup.
func refDefPrepareRange(source []byte, line int, _ string) (prepareRenameResult, bool) {
	lines := splitLines(source)
	if line-1 >= len(lines) {
		return prepareRenameResult{}, false
	}
	row := lines[line-1]
	m := refDefBracketBytes(row)
	if m == nil {
		return prepareRenameResult{}, false
	}
	startCh := utf16FromByteOffset(row, m[0])
	endCh := utf16FromByteOffset(row, m[1])
	return prepareRenameResult{
		Range: Range{
			Start: Position{Line: line - 1, Character: startCh},
			End:   Position{Line: line - 1, Character: endCh},
		},
		Placeholder: string(row[m[0]:m[1]]),
	}, true
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
	close := -1
	for j := open; j < len(row); j++ {
		if row[j] == ']' {
			close = j
			break
		}
	}
	if close < 0 || close == open {
		return nil
	}
	// After `]` we need `:` to qualify as a definition.
	if close+1 >= len(row) || row[close+1] != ':' {
		return nil
	}
	return []int{open, close}
}

// refUsePrepareRange builds the rename range for a reference-style
// link use (`[text][label]`, `[label][]`, or `[label]`). The cursor
// position determines whether the user is editing the label or the
// text — both are valid rename surfaces, and both edit the same
// label. The placeholder reflects the document's raw bracket
// content (preserving casing and spacing) so the rename popup
// pre-fills with what the user sees in the buffer, not the
// normalized form that powers cross-link matching.
func refUsePrepareRange(source []byte, line, col int, label string) (prepareRenameResult, bool) {
	lines := splitLines(source)
	if line-1 >= len(lines) {
		return prepareRenameResult{}, false
	}
	row := lines[line-1]
	startByte, endByte, ok := refUseLabelBytes(row, col-1, label)
	if !ok {
		return prepareRenameResult{}, false
	}
	startCh := utf16FromByteOffset(row, startByte)
	endCh := utf16FromByteOffset(row, endByte)
	return prepareRenameResult{
		Range: Range{
			Start: Position{Line: line - 1, Character: startCh},
			End:   Position{Line: line - 1, Character: endCh},
		},
		Placeholder: string(row[startByte:endByte]),
	}, true
}

// refUseLabelBytes scans row for the bracket pair that holds the
// label of a reference-style link covering cursorByte. Returns the
// label content range. For full `[text][label]` it returns the
// second bracket pair; for shortcut `[label]` and collapsed
// `[label][]` it returns the only/first bracket pair. The label
// argument is the normalized label (lowercase, whitespace
// collapsed); matching is done against the raw bracket content via
// the same util.ToLinkReference normalization.
func refUseLabelBytes(row []byte, cursorByte int, label string) (int, int, bool) {
	pairs := bracketPairs(row)
	for i, pr := range pairs {
		if cursorByte < pr.open || cursorByte > pr.close {
			continue
		}
		if start, end, ok := matchLeadingPair(row, pairs, i, label); ok {
			return start, end, true
		}
		if start, end, ok := matchTrailingPair(row, pairs, i, label); ok {
			return start, end, true
		}
		// Shortcut `[label]`: this pair's content normalizes to label.
		if normalizedLabel(row[pr.open+1:pr.close]) == label {
			return pr.open + 1, pr.close, true
		}
	}
	return 0, 0, false
}

// matchLeadingPair handles the case where the cursor sits in the
// leading pair of a reference link. Returns the label range when
// pairs[i] is followed by a flush pair: a full `[text][label]`
// resolves to the trailing pair, while a collapsed `[label][]`
// resolves to the leading pair (the trailing one is empty).
func matchLeadingPair(row []byte, pairs []bracketPair, i int, label string) (int, int, bool) {
	if i+1 >= len(pairs) {
		return 0, 0, false
	}
	next := pairs[i+1]
	pr := pairs[i]
	if next.open != pr.close+1 {
		return 0, 0, false
	}
	if normalizedLabel(row[next.open+1:next.close]) == label {
		return next.open + 1, next.close, true
	}
	if next.close == next.open+1 && normalizedLabel(row[pr.open+1:pr.close]) == label {
		return pr.open + 1, pr.close, true
	}
	return 0, 0, false
}

// matchTrailingPair handles the case where the cursor sits in the
// trailing pair of a reference link. The previous pair sits flush
// before our opener; for collapsed references the trailing pair is
// empty and the label lives in the previous pair, while for full
// references the trailing pair carries the label directly.
func matchTrailingPair(row []byte, pairs []bracketPair, i int, label string) (int, int, bool) {
	if i == 0 {
		return 0, 0, false
	}
	prev := pairs[i-1]
	pr := pairs[i]
	if prev.close+1 != pr.open {
		return 0, 0, false
	}
	if pr.close == pr.open+1 && normalizedLabel(row[prev.open+1:prev.close]) == label {
		return prev.open + 1, prev.close, true
	}
	if normalizedLabel(row[pr.open+1:pr.close]) == label {
		return pr.open + 1, pr.close, true
	}
	return 0, 0, false
}

func normalizedLabel(b []byte) string {
	return string(util.ToLinkReference(b))
}

// invalidLinkRefRune returns the first rune in s that would make
// the resulting `[label]: …` line unparsable, or 0 when s is safe.
// Newlines and bare brackets are the canonical breakers — both
// terminate the label run before the literal text the user typed.
func invalidLinkRefRune(s string) rune {
	for _, r := range s {
		switch r {
		case '\n', '\r', '[', ']':
			return r
		}
	}
	return 0
}

// handleRename answers textDocument/rename. The reply is a
// WorkspaceEdit that covers every affected file. Heading rename
// rewrites incoming anchor links across the workspace; link-ref
// label rename rewrites the def and every use in the same file.
// Collisions return InvalidParams with renameCollisionData so the
// client can show a meaningful error instead of partially applying
// an edit.
func (s *Server) handleRename(msg *requestMessage) {
	var p renameParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid rename params")
		return
	}
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, nil)
		return
	}
	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)
	switch res.Tag {
	case index.TokenHeading:
		s.renameHeading(msg, p, source, rel, line, res, p.NewName)
	case index.TokenRefDef, index.TokenRefUse:
		s.renameLinkRef(msg, p, source, res.Label, p.NewName)
	default:
		_ = s.t.writeError(msg.ID, codeInvalidParams, "rename not supported at this position")
	}
}

// renameHeading rewrites a heading and every workspace anchor link
// pointing at it.
//
// Algorithm:
//  1. Recompute the file's heading slug map under the new heading
//     text using mdtext.CollectTOCItems.
//  2. Reject the rename when its new bare slug collides with another
//     heading's bare slug (a *new* duplicate would force the
//     disambiguator to shift in surprising ways; cross-file links
//     would silently break).
//  3. Compare old and new slug maps to identify shifted headings.
//  4. Build TextEdits: replace the heading text on the source line
//     and rewrite every incoming anchor link for each (file,
//     oldSlug → newSlug) pair.
func (s *Server) renameHeading(
	msg *requestMessage, p renameParams,
	source []byte, rel string, line int, res index.LocateResult, newName string,
) {
	idx := s.ensureIndex()
	oldText := res.Name
	if strings.TrimSpace(newName) == strings.TrimSpace(oldText) {
		_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: map[string][]textEdit{}})
		return
	}
	// Reject a new heading text that slugifies to nothing (e.g.
	// punctuation-only). The renamed heading would have no
	// addressable anchor — CollectTOCItems and the index's heading
	// walk both skip empty slugs — and the per-edge rewrite would
	// produce `#` placeholders that break every incoming link
	// instead of redirecting them.
	if res.Anchor != "" && mdtext.Slugify(newName) == "" {
		_ = s.t.writeError(msg.ID, codeInvalidParams,
			"new heading text has no addressable slug; pick text containing letters or digits")
		return
	}
	oldSlugs, newSlugs, conflict := computeSlugRemap(source, line, oldText, newName)
	if conflict != "" {
		_ = s.t.writeErrorWithData(msg.ID, codeInvalidParams,
			"rename would collide with heading "+conflict,
			renameCollisionData{Conflict: conflict})
		return
	}
	headingEdit, ok := headingTextEdit(source, line, newName)
	if !ok {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "cannot locate heading text on line")
		return
	}
	changes := map[string][]textEdit{p.TextDocument.URI: {headingEdit}}
	for old, new := range slugRemapPairs(oldSlugs, newSlugs) {
		if old == "" || old == new {
			continue
		}
		s.appendAnchorEditsForHeading(changes, idx, rel, old, new)
	}
	stableSortEdits(changes)
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: changes})
}

// computeSlugRemap returns the file's old slug list, its new slug
// list (after substituting newText for the heading on `line`), and a
// non-empty `conflict` heading name when the rename would create a
// new duplicate base slug. The returned slices share index ordering:
// oldSlugs[i] is the slug of the i-th heading before rename and
// newSlugs[i] is its slug after.
//
// The walk includes every heading, including ones whose slug is
// empty (punctuation-only text). Skipping them — as
// mdtext.CollectTOCItems does — would desynchronize the per-heading
// indices from the heading-line walk and mis-identify the renamed
// heading on files with empty-slug headings before the cursor's
// line. It would also block the case of renaming an empty-slug
// heading to a real one (which can shift later disambiguators).
func computeSlugRemap(source []byte, line int, oldText, newText string) ([]string, []string, string) {
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
	texts[target] = newText
	// Collision check: any other heading shares the new bare slug?
	newBase := mdtext.Slugify(newText)
	if newBase != "" {
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
// node. Carries no slug — the slug is computed in lockstep with the
// rename so empty-slug headings don't get silently dropped.
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

// assignSlugs runs the same disambiguator pass mdtext.CollectTOCItems
// uses, but operates on a parallel slice of texts so callers can
// substitute a renamed heading's text in place without losing
// alignment with the heading walk. Headings whose base slug is
// empty stay at "" — those have no anchor and never participate in
// link rewrites.
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

// slugRemapPairs returns a map from old slug to new slug for every
// heading whose slug changed. Skips entries whose old slug is empty
// (headings with no slug, e.g. punctuation-only).
func slugRemapPairs(oldSlugs, newSlugs []string) map[string]string {
	out := map[string]string{}
	for i := range oldSlugs {
		if oldSlugs[i] == "" || oldSlugs[i] == newSlugs[i] {
			continue
		}
		// First wins when two old slugs map to the same new slug —
		// shouldn't happen in practice because the disambiguator
		// keeps slugs unique, but the guard prevents an inflated
		// rewrite from a corrupted index.
		if _, exists := out[oldSlugs[i]]; !exists {
			out[oldSlugs[i]] = newSlugs[i]
		}
	}
	return out
}

// headingTextEdit replaces the heading text on the source line with
// newName. Returns false when the line is not recognized as a
// heading line.
func headingTextEdit(source []byte, line int, newName string) (textEdit, bool) {
	lines := splitLines(source)
	if line-1 >= len(lines) {
		return textEdit{}, false
	}
	row := lines[line-1]
	startByte, endByte, ok := atxHeadingTextByteRange(row)
	if !ok {
		startByte, endByte = trimmedRange(row)
	}
	startCh := utf16FromByteOffset(row, startByte)
	endCh := utf16FromByteOffset(row, endByte)
	return textEdit{
		Range: Range{
			Start: Position{Line: line - 1, Character: startCh},
			End:   Position{Line: line - 1, Character: endCh},
		},
		NewText: newName,
	}, true
}

// appendAnchorEditsForHeading records one TextEdit per workspace
// anchor link pointing at (headingFile, oldSlug). The new slug is
// substituted into each link destination's fragment portion; the
// path component is left unchanged so relative includes (`./other.md`
// etc.) keep their source form.
func (s *Server) appendAnchorEditsForHeading(
	changes map[string][]textEdit, idx *index.Index,
	headingFile, oldSlug, newSlug string,
) {
	edges := idx.IncomingEdges(headingFile, oldSlug)
	for _, e := range edges {
		uri, edit, ok := s.anchorEditForEdge(e, oldSlug, newSlug)
		if !ok {
			continue
		}
		changes[uri] = append(changes[uri], edit)
	}
}

// anchorEditForEdge converts one incoming-edge record into a
// concrete TextEdit on the source file. Returns false when the
// source file isn't readable (out of workspace, deleted) or when
// the edge's link can't be located in the source — the rename
// silently skips those rather than failing the whole request, since
// the alternative would block a user from renaming a heading because
// of an unrelated stale edge.
func (s *Server) anchorEditForEdge(e index.Edge, oldSlug, newSlug string) (string, textEdit, bool) {
	uri := s.workspaceURI(e.SourceFile)
	if uri == "" {
		return "", textEdit{}, false
	}
	source, _, ok := s.docTextOrFile(uri)
	if !ok {
		return "", textEdit{}, false
	}
	lines := splitLines(source)
	if e.SourceLine < 1 || e.SourceLine > len(lines) {
		return "", textEdit{}, false
	}
	row := lines[e.SourceLine-1]
	startByte, endByte, ok := anchorFragmentBytes(row, e.SourceCol-1, oldSlug)
	if !ok {
		return "", textEdit{}, false
	}
	startCh := utf16FromByteOffset(row, startByte)
	endCh := utf16FromByteOffset(row, endByte)
	return uri, textEdit{
		Range: Range{
			Start: Position{Line: e.SourceLine - 1, Character: startCh},
			End:   Position{Line: e.SourceLine - 1, Character: endCh},
		},
		NewText: newSlug,
	}, true
}

// anchorFragmentBytes locates the byte range of the fragment slug
// inside a link destination on row, starting from the link's
// text-start column. Returns the range of the raw fragment text
// (excluding the leading `#`) so callers can replace it with the
// new slug.
//
// The match is normalized: the raw fragment is URL-unescaped and
// run through mdtext.Slugify, mirroring the way the index keys
// incoming edges. That way `(#Setup)` and `(#Docs%20API)` both
// participate in a rename even though their literal byte
// sequences differ from `setup` / `docs-api`.
func anchorFragmentBytes(row []byte, textStart int, oldSlug string) (int, int, bool) {
	bracketStart := textStart
	if bracketStart < 0 {
		bracketStart = 0
	}
	if bracketStart >= len(row) {
		return 0, 0, false
	}
	open, close, ok := destBounds(row, bracketStart)
	if !ok {
		return 0, 0, false
	}
	hash := indexOfHash(row, open, close)
	if hash < 0 {
		return 0, 0, false
	}
	fragEnd := fragmentEnd(row, hash+1, close)
	rawFrag := row[hash+1 : fragEnd]
	if !fragmentMatchesSlug(rawFrag, oldSlug) {
		return 0, 0, false
	}
	return hash + 1, fragEnd, true
}

// destBounds returns the `(` open and `)` close byte offsets of a
// link destination starting at or after `from` on row, accounting
// for nested parens. Backslash-escaped parens are treated as
// literal bytes (CommonMark allows `\(` / `\)` inside the
// destination), so a link like `[t](foo\(bar\)#sec)` resolves to
// the outer parens, not the escaped inner ones.
func destBounds(row []byte, from int) (int, int, bool) {
	open := -1
	for i := from; i < len(row)-1; i++ {
		if row[i] == ']' && row[i+1] == '(' {
			open = i + 2
			break
		}
	}
	if open < 0 {
		return 0, 0, false
	}
	close := -1
	depth := 1
	for j := open; j < len(row) && close < 0; j++ {
		if isBackslashEscaped(row, j) {
			continue
		}
		switch row[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = j
			}
		}
	}
	if close < 0 {
		return 0, 0, false
	}
	return open, close, true
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
// or -1 when the destination has no fragment. The first `#` wins
// because URL fragments by definition start at the first `#`.
func indexOfHash(row []byte, open, close int) int {
	for i := open; i < close; i++ {
		if row[i] == '#' {
			return i
		}
	}
	return -1
}

// fragmentEnd returns the byte offset where a fragment ends within
// row[start:close]. CommonMark inline-link destinations don't carry
// query strings (the `(url "title")` title form is parsed by
// goldmark separately), so the fragment runs from `start` to the
// first whitespace, the first `>` (CommonMark's angle-bracketed
// destination form `<url>`), or to `close`. Without the `>` guard,
// `[t](<#sec>)` would slugify `sec>` as `sec` and the rename would
// then overwrite the closing bracket.
func fragmentEnd(row []byte, start, close int) int {
	for i := start; i < close; i++ {
		if row[i] == ' ' || row[i] == '\t' || row[i] == '>' {
			return i
		}
	}
	return close
}

// fragmentMatchesSlug reports whether the raw bytes of a link
// fragment slugify to oldSlug. URL-unescape mirrors decodeAnchor
// in the index so `%20` and friends decode the same way before
// the slug pass.
func fragmentMatchesSlug(rawFrag []byte, oldSlug string) bool {
	decoded, err := url.PathUnescape(string(rawFrag))
	if err != nil {
		decoded = string(rawFrag)
	}
	return mdtext.Slugify(decoded) == oldSlug
}

// stableSortEdits sorts each file's TextEdit slice by start
// position. The LSP spec leaves ordering unspecified but most
// clients require non-overlapping edits applied bottom-up; sorting
// keeps the output deterministic for tests and lets the client (or
// a fallback bottom-up applier in our integration tests) apply
// edits in reverse order without surprises.
func stableSortEdits(changes map[string][]textEdit) {
	for uri, edits := range changes {
		sort.SliceStable(edits, func(i, j int) bool {
			a, b := edits[i].Range.Start, edits[j].Range.Start
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return a.Character < b.Character
		})
		changes[uri] = edits
	}
}

// renameLinkRef rewrites a link-reference def and every use of the
// label in the same file.
//
// Collision handling: a new label that matches another existing def
// in the file fails with InvalidParams — MDS028 / MDS029 would
// surface the breakage on the next lint pass anyway, but the LSP
// error catches it before the edit applies.
func (s *Server) renameLinkRef(
	msg *requestMessage, p renameParams,
	source []byte, oldLabel, newName string,
) {
	if strings.TrimSpace(newName) == "" {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "label cannot be empty")
		return
	}
	// Reject labels that would break the on-disk syntax. A bare
	// `]` would close the bracket pair early, producing an
	// unparsable `[label]:` line; a newline or `[` similarly
	// destroys the def. CommonMark allows escapes (`\]`), but
	// emitting an escaped form here would silently rewrite the
	// user's typed text — forcing the user to pick a valid label
	// is the safer path.
	if invalid := invalidLinkRefRune(newName); invalid != 0 {
		_ = s.t.writeError(msg.ID, codeInvalidParams,
			fmt.Sprintf("label cannot contain %q", invalid))
		return
	}
	// Don't early-return when the normalized label is unchanged.
	// A rename from `docs api` to `Docs API` keeps the same
	// normalized form but changes the visible spelling; users can
	// legitimately ask for that to refresh casing or whitespace
	// across the def and every use. labelConflict still uses the
	// normalized form for collision matching so a same-normal-form
	// rename can never trip the collision check against itself.
	newLabel := normalizedLabel([]byte(newName))
	if conflict := labelConflict(source, oldLabel, newLabel); conflict != "" {
		_ = s.t.writeErrorWithData(msg.ID, codeInvalidParams,
			"rename would collide with link reference ["+conflict+"]",
			renameCollisionData{Conflict: conflict})
		return
	}
	edits := linkRefEdits(source, oldLabel, newName)
	if len(edits) == 0 {
		_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: map[string][]textEdit{}})
		return
	}
	changes := map[string][]textEdit{p.TextDocument.URI: edits}
	stableSortEdits(changes)
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: changes})
}

// labelConflict returns the conflicting label name (preserving the
// original casing) when newLabel matches a link-reference
// definition in the file other than the one being renamed. Empty
// string means "no conflict".
//
// The scan goes through the source directly rather than through
// idx.File(rel).Symbols, because the index intentionally
// deduplicates link-reference definitions: only the first def per
// normalized label survives. Trusting the index would let a rename
// pass collision when the buffer carries a duplicate `[label]: …`
// line for newLabel — silently producing two coexisting defs that
// MDS028 / MDS029 would have to clean up later.
func labelConflict(source []byte, oldLabel, newLabel string) string {
	body, _ := bodyAndFMOffset(source)
	for _, m := range index.RefDefRegexpMatches(body) {
		raw := body[m[2]:m[3]]
		norm := normalizedLabel(raw)
		if norm == oldLabel {
			// Defs that match the rename's source label are the
			// ones being rewritten — they don't count as collisions.
			continue
		}
		if norm == newLabel {
			return string(raw)
		}
	}
	return ""
}

// linkRefEdits walks the source for the def line and every
// reference-style use of oldLabel (full and shortcut), returning
// one TextEdit per match. The def is rewritten via refDefBracketBytes;
// uses are rewritten via the link AST walk so the precise label
// brackets are targeted regardless of whether the link is full,
// shortcut, or collapsed.
func linkRefEdits(source []byte, oldLabel, newName string) []textEdit {
	body, fmOffset := bodyAndFMOffset(source)
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	lines := splitLines(source)
	var out []textEdit
	out = append(out, refDefEditsInBody(body, lines, fmOffset, oldLabel, newName)...)
	out = append(out, refUseEditsInBody(root, body, lines, fmOffset, oldLabel, newName)...)
	return out
}

// refDefEditsInBody finds the `[label]: url` line(s) for oldLabel
// and emits one TextEdit per match. Goldmark only resolves the first
// definition for a given label, but the file may legally carry
// duplicate def lines (the rest are unused). We rewrite all of them
// so the file ends up internally consistent.
func refDefEditsInBody(
	body []byte, lines [][]byte, fmOffset int,
	oldLabel, newName string,
) []textEdit {
	var out []textEdit
	for _, m := range refDefMatches(body) {
		raw := body[m[2]:m[3]]
		if normalizedLabel(raw) != oldLabel {
			continue
		}
		bodyLine := lineOfBodyOffset(body, m[2])
		fileLine := bodyLine + fmOffset
		if fileLine-1 >= len(lines) {
			continue
		}
		row := lines[fileLine-1]
		bracket := refDefBracketBytes(row)
		if bracket == nil {
			continue
		}
		startCh := utf16FromByteOffset(row, bracket[0])
		endCh := utf16FromByteOffset(row, bracket[1])
		out = append(out, textEdit{
			Range: Range{
				Start: Position{Line: fileLine - 1, Character: startCh},
				End:   Position{Line: fileLine - 1, Character: endCh},
			},
			NewText: newName,
		})
	}
	return out
}

// refDefMatches wraps refDefRE.FindAllSubmatchIndex so the caller
// can iterate without repeating the regex name.
func refDefMatches(body []byte) [][]int {
	return index.RefDefRegexpMatches(body)
}

// refUseEditsInBody walks the AST for ast.Link nodes whose
// Reference matches oldLabel and emits one TextEdit per use.
// Full `[text][label]` rewrites the second pair of brackets;
// shortcut `[label]` rewrites the only pair; collapsed `[text][]`
// keeps the empty `[]` and rewrites the first (text) pair, since
// the text doubles as the label.
func refUseEditsInBody(
	root ast.Node, body []byte, lines [][]byte, fmOffset int,
	oldLabel, newName string,
) []textEdit {
	// Build the line-offset table once. Without this, refUseEdit's
	// per-link line lookups would be O(n) each; on a file with N
	// reference uses the cost grew to O(N·n).
	idx := newBodyLineIndex(body)
	var out []textEdit
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

// refUseEdit converts one link node into a TextEdit. Returns false
// when the source position can't be recovered (e.g. nested in an
// unsupported block) so the caller can skip the use rather than
// emit a corrupt edit.
func refUseEdit(
	l *ast.Link, body []byte, lines [][]byte, fmOffset int, newName string,
	bodyIdx bodyLineIndex,
) (textEdit, bool) {
	textStart, textEnd := linkTextBounds(l, body)
	if textStart < 0 {
		return textEdit{}, false
	}
	bodyLine := bodyIdx.LineOfOffset(textStart)
	fileLine := bodyLine + fmOffset
	if fileLine-1 >= len(lines) {
		return textEdit{}, false
	}
	row := lines[fileLine-1]
	lineStart := bodyIdx.LineStart(bodyLine)
	if lineStart < 0 {
		return textEdit{}, false
	}
	textOpenCol := textStart - lineStart - 1 // include the `[`
	if textOpenCol < 0 {
		textOpenCol = 0
	}
	textCloseCol := textEnd - lineStart // points just past last text byte
	pairs := bracketPairs(row)
	first, second := matchingPair(pairs, textOpenCol, textCloseCol)
	if first.open < 0 {
		return textEdit{}, false
	}
	target := first
	switch l.Reference.Type {
	case ast.ReferenceLinkFull:
		if second.open < 0 {
			return textEdit{}, false
		}
		target = second
	case ast.ReferenceLinkCollapsed:
		// Collapsed `[text][]` — text doubles as the label.
		// Rewrite the first pair.
	default:
		// Shortcut: only the first pair exists; rewrite it.
	}
	startCh := utf16FromByteOffset(row, target.open+1)
	endCh := utf16FromByteOffset(row, target.close)
	return textEdit{
		Range: Range{
			Start: Position{Line: fileLine - 1, Character: startCh},
			End:   Position{Line: fileLine - 1, Character: endCh},
		},
		NewText: newName,
	}, true
}

// linkTextBounds returns the [start, end) absolute byte offsets of
// the link's display-text run inside body. End is one past the last
// character of the visible text (i.e. the position of the closing
// `]`). Returns (-1, -1) when the link has no parsed text segment.
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

// bracketPairs returns every `[` / `]` pair on row, in left-to-right
// order. Escaped `\]` is honored. The function only tracks pairs at
// the row level; nested brackets inside images/links are intentionally
// not modeled because reference labels never nest in CommonMark.
type bracketPair struct{ open, close int }

func bracketPairs(row []byte) []bracketPair {
	var pairs []bracketPair
	for i := 0; i < len(row); i++ {
		if row[i] != '[' {
			continue
		}
		for j := i + 1; j < len(row); j++ {
			if row[j] == '\\' && j+1 < len(row) {
				j++
				continue
			}
			if row[j] == ']' {
				pairs = append(pairs, bracketPair{i, j})
				i = j
				break
			}
		}
	}
	return pairs
}

// matchingPair returns the (text, label) bracket pair touching the
// link whose text-bracket bounds are [textOpenCol, textCloseCol].
// The first returned pair is the text bracket; the second is the
// label bracket if the next pair starts immediately after the text
// pair's `]` (full or collapsed reference). When no second pair is
// present, the label is the only pair (shortcut reference).
func matchingPair(pairs []bracketPair, textOpenCol, textCloseCol int) (bracketPair, bracketPair) {
	miss := bracketPair{open: -1, close: -1}
	first, second := miss, miss
	for i, pr := range pairs {
		// The text bracket starts at textOpenCol-ish (the link's
		// `[` is one byte before the first text segment). Allow a
		// small slack because escapes may shift the text segment.
		if pr.open <= textOpenCol && pr.close >= textCloseCol-1 {
			first = pr
			if i+1 < len(pairs) && pairs[i+1].open == pr.close+1 {
				second = pairs[i+1]
			}
			return first, second
		}
	}
	return first, second
}

// lineOfBodyOffset returns the 1-based line of byte offset off
// within body. The two callers (computeSlugRemap and
// refDefEditsInBody) each invoke it once per heading or def, so
// the linear scan is bounded; tight per-edit loops use
// bodyLineIndex instead to avoid quadratic behavior.
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

// bodyLineIndex precomputes the byte offset of every line start in
// a body so lookups for line→offset and offset→line run in O(log n)
// instead of O(n) per call. Use it when a single rename emits many
// per-link edits; without it, refUseEdit's per-edit scans were
// quadratic in the number of reference uses.
type bodyLineIndex struct {
	starts []int
}

// newBodyLineIndex builds a line-start table for body. The first
// entry is always 0 (start of line 1); each `\n` advances to the
// next line's start.
func newBodyLineIndex(body []byte) bodyLineIndex {
	starts := make([]int, 1, 1+bodyNewlineCount(body))
	for i, b := range body {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return bodyLineIndex{starts: starts}
}

// bodyNewlineCount returns the count of `\n` bytes in body. Used to
// presize the line-start slice without an extra alloc cycle.
func bodyNewlineCount(body []byte) int {
	n := 0
	for _, b := range body {
		if b == '\n' {
			n++
		}
	}
	return n
}

// LineOfOffset returns the 1-based line number containing byte off.
// Out-of-range offsets clamp to the boundary line.
func (b bodyLineIndex) LineOfOffset(off int) int {
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

// LineStart returns the byte offset of the start of 1-based line n,
// or -1 when n falls outside the table.
func (b bodyLineIndex) LineStart(n int) int {
	if n < 1 || n > len(b.starts) {
		return -1
	}
	return b.starts[n-1]
}

// bodyAndFMOffset splits source into body and the line offset the
// front matter contributed. Mirrors the slicing the index does so
// renames work consistently against files with or without front
// matter.
func bodyAndFMOffset(source []byte) ([]byte, int) {
	fm, body := lint.StripFrontMatter(source)
	off := 0
	if len(fm) > 0 {
		for _, b := range fm {
			if b == '\n' {
				off++
			}
		}
	}
	return body, off
}
