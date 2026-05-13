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
		// Locator's regex tags any `[label]: url`-looking line as
		// a def, including matches inside fenced code blocks. Gate
		// on the parser-validated set so the rename popup never
		// surfaces on code samples.
		if !isValidRefDefLine(source, line) {
			return prepareRenameResult{}, false
		}
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

// isValidRefDefLine reports whether a 1-based source line holds a
// real reference definition (one goldmark accepted), translating
// the source-coordinate line into body coordinates so the
// validRefDefBodyLines lookup matches.
func isValidRefDefLine(source []byte, line int) bool {
	body, fmOffset := bodyAndFMOffset(source)
	bodyLine := line - fmOffset
	if bodyLine < 1 {
		return false
	}
	return validRefDefBodyLines(body)[bodyLine]
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
	// trimTrailingHashRun never erodes past textStart — the bounded
	// `for k > start` loop and the explicit `k > start` guard before
	// returning trimRightSpace(row, start, k-1) keep end >= textStart.
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

// firstControlRune returns the first newline / carriage return in
// s, or 0 when the string is a single line. Used to keep
// single-line surfaces (heading text, link-ref labels) from being
// rewritten into multi-line garbage.
func firstControlRune(s string) rune {
	for _, r := range s {
		if r == '\n' || r == '\r' {
			return r
		}
	}
	return 0
}

// invalidLinkRefRune returns the first rune in s that would make
// the resulting `[label]: …` line unparsable, or 0 when s is safe.
//
// Newlines force the label run onto a second line where the def
// no longer parses. `]` ends the label early, leaving the rest
// as paragraph text. `[` is technically permitted via escape
// (`\[`), but emitting a raw `[` here would still confuse most
// CommonMark renderers and our own ref-def regex, so we forbid
// both bracket forms outright rather than auto-escaping the user's
// typed text.
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
	case index.TokenRefDef:
		// Mirror prepareRename's gate: a `[label]: url`-shaped
		// line inside a fenced code block or PI body isn't a
		// real def, so refuse the rename rather than producing
		// empty / off-target edits.
		if !isValidRefDefLine(source, line) {
			_ = s.t.writeError(msg.ID, codeInvalidParams, "rename not supported at this position")
			return
		}
		s.renameLinkRef(msg, p, source, res.Label, p.NewName)
	case index.TokenRefUse:
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
//     text. The pass mirrors mdtext.CollectTOCItems' disambiguator
//     logic via assignSlugs, but walks every heading (including
//     empty-slug ones CollectTOCItems would skip) so the slug
//     map stays aligned with the rename's heading walk.
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
	// Reject newName values that would corrupt the heading line.
	// A `\n` or `\r` turns a single-line heading into a multi-line
	// insertion, splitting the document at the source location.
	if r := firstControlRune(newName); r != 0 {
		_ = s.t.writeError(msg.ID, codeInvalidParams,
			fmt.Sprintf("heading text cannot contain %q", r))
		return
	}
	// Reject a new heading text that slugifies to nothing (e.g.
	// punctuation-only). The renamed heading would have no
	// addressable anchor — CollectTOCItems and the index's heading
	// walk both skip empty slugs — so cross-file links would have
	// nowhere to land. The check is unconditional so the behavior
	// matches the docs: any rename to an empty-slug name fails,
	// regardless of whether the old heading had a slug.
	if mdtext.Slugify(newName) == "" {
		_ = s.t.writeError(msg.ID, codeInvalidParams,
			"new heading text has no addressable slug; pick text containing letters or digits")
		return
	}
	oldSlugs, newSlugs, conflict := computeSlugRemap(source, line, newName)
	if conflict != "" {
		_ = s.t.writeErrorWithData(msg.ID, codeInvalidParams,
			"rename would collide with heading "+conflict,
			renameCollisionData{Conflict: conflict})
		return
	}
	// headingTextEdit is reachable only when the cursor's line
	// is inside the source — the locator would have returned
	// TokenNone otherwise — so the false branch is unreachable
	// here.
	headingEdit, _ := headingTextEdit(source, line, newName)
	changes := map[string][]textEdit{p.TextDocument.URI: {headingEdit}}
	for old, new := range slugRemapPairs(oldSlugs, newSlugs) {
		// slugRemapPairs already filters empty and unchanged
		// slugs, so no redundant guard is needed here.
		s.appendAnchorEditsForHeading(changes, idx, rel, old, new)
		// Ref-def destinations like `[label]: ./a.md#slug` aren't
		// recorded as edges in the index (defs are symbols, not
		// edges; only their uses become EdgeRefLink). Walk every
		// workspace file once per shifted slug to find ref-defs
		// whose destination resolves to the renamed heading.
		s.appendRefDefDestEditsForHeading(changes, idx, rel, old, new)
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
	// Skip the check when the rename keeps the same base slug —
	// a non-semantic edit (case, punctuation) inside an existing
	// duplicate-name group doesn't *introduce* a new collision,
	// so blocking it would surprise the user.
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

// appendRefDefDestEditsForHeading rewrites `[label]: url`
// definitions whose destination URL points at the renamed
// heading. The index treats ref-defs as symbols rather than
// edges, so appendAnchorEditsForHeading skips them — without
// this companion pass a heading rename would leave def lines
// like `[setup]: ./a.md#setup` stranded on the old slug while
// every `[t][setup]` use still resolved correctly through the
// def to a now-stale anchor.
//
// The pass iterates every file the index knows about, reads
// each through resolveURIAndSource so unsaved buffers win
// over disk, and emits one edit per def whose destination
// URL fragment slugifies to oldSlug AND whose path resolves
// to headingFile (or is the same file when the def is anchor-
// only).
func (s *Server) appendRefDefDestEditsForHeading(
	changes map[string][]textEdit, idx *index.Index,
	headingFile, oldSlug, newSlug string,
) {
	headingFile = index.NormalizePath(headingFile)
	for _, rel := range idx.Files() {
		uri, source, ok := s.resolveURIAndSource(rel)
		if !ok {
			continue
		}
		body, fmOffset := bodyAndFMOffset(source)
		fileLines := splitLines(source)
		// Route through validRefDefMatches so `[label]:` lines
		// goldmark refused (code blocks, paragraph continuations)
		// don't get rewritten as if they were real defs.
		for _, m := range validRefDefMatches(body) {
			edit, ok := refDefDestEditForMatch(
				body, fileLines, fmOffset, m.matchIdx,
				rel, headingFile, oldSlug, newSlug,
			)
			if !ok {
				continue
			}
			changes[uri] = append(changes[uri], edit)
		}
	}
}

// refDefDestEditForMatch turns one regex match for a
// `[label]: url` line into a TextEdit on the URL's slug
// portion, or returns ok=false when the destination doesn't
// point at the renamed heading.
//
// The URL portion is everything after `]:` on the line,
// trimmed of surrounding whitespace and an optional quoted
// title. We don't need to be perfect about the title syntax
// — we just need to locate the `#slug` substring inside the
// destination part, the same way anchorFragmentBytes locates
// it inside an inline link.
func refDefDestEditForMatch(
	body []byte, fileLines [][]byte, fmOffset int, m []int,
	defFile, headingFile, oldSlug, newSlug string,
) (textEdit, bool) {
	bodyLine := lineOfBodyOffset(body, m[2])
	fileLine := bodyLine + fmOffset
	if fileLine-1 >= len(fileLines) {
		return textEdit{}, false
	}
	row := fileLines[fileLine-1]
	colonOff := refDefColonOffset(row)
	if colonOff < 0 {
		return textEdit{}, false
	}
	destStart, destEnd := refDefDestRange(row, colonOff+1)
	if destStart >= destEnd {
		return textEdit{}, false
	}
	dest := row[destStart:destEnd]
	if !refDefDestPointsAt(dest, defFile, headingFile, oldSlug) {
		return textEdit{}, false
	}
	// refDefDestPointsAt already confirmed the destination
	// contains `#oldSlug` (oldSlug is non-empty when the rename
	// reaches this path), so `#` always exists in dest.
	hashIdx := destStart
	for hashIdx < destEnd && row[hashIdx] != '#' {
		hashIdx++
	}
	fragEnd := fragmentEnd(row, hashIdx+1, destEnd)
	startCh := utf16FromByteOffset(row, hashIdx+1)
	endCh := utf16FromByteOffset(row, fragEnd)
	return textEdit{
		Range: Range{
			Start: Position{Line: fileLine - 1, Character: startCh},
			End:   Position{Line: fileLine - 1, Character: endCh},
		},
		NewText: newSlug,
	}, true
}

// refDefColonOffset returns the byte offset of the `:` that
// follows the `[label]` in a ref-def line, or -1 when the
// shape doesn't match. Mirrors the regex's expectation: ≤3
// leading spaces, then `[label]:` where label has no `]`.
func refDefColonOffset(row []byte) int {
	i := 0
	for i < len(row) && i < 3 && row[i] == ' ' {
		i++
	}
	if i >= len(row) || row[i] != '[' {
		return -1
	}
	close := -1
	for j := i + 1; j < len(row); j++ {
		if row[j] == ']' {
			close = j
			break
		}
	}
	if close < 0 || close+1 >= len(row) || row[close+1] != ':' {
		return -1
	}
	return close + 1
}

// refDefDestRange returns the byte range of the destination
// URL portion of a ref-def line, given the offset just past
// the `:`. Skips leading whitespace, then handles both forms:
//
//   - Angle-bracketed: `[label]: <url>`. The returned range
//     covers the bytes *inside* the angle brackets, so a slug
//     edit replaces only the fragment text and leaves `<` / `>`
//     intact.
//   - Bare: `[label]: url`. The range runs from the first
//     non-whitespace byte to the next whitespace.
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
		// Unterminated `<…` — fall through to the bare reader
		// from the original `<` position so a malformed line
		// doesn't masquerade as an angle-bracketed dest.
	}
	start := i
	for i < len(row) && row[i] != ' ' && row[i] != '\t' {
		i++
	}
	return start, i
}

// refDefDestPointsAt reports whether the dest bytes (URL
// portion of a ref-def in defFile) resolve to (headingFile,
// oldSlug). The check mirrors the index's edge collector:
// percent-decode the fragment, run through mdtext.Slugify,
// and compare against oldSlug; resolve the path against
// defFile's directory and compare against headingFile.
func refDefDestPointsAt(dest []byte, defFile, headingFile, oldSlug string) bool {
	t, ok := refDefParseTarget(string(dest))
	if !ok {
		return false
	}
	// url.Parse already produced t.fragment in decoded form, but
	// re-running PathUnescape mirrors the index's `decodeAnchor`
	// helper so corner-case escapes can't drift between the two.
	// Errors are impossible at this point: url.Parse would have
	// rejected the dest if it had a malformed `%xx` sequence.
	decoded, _ := url.PathUnescape(t.fragment)
	if mdtext.Slugify(decoded) != oldSlug {
		return false
	}
	if t.localAnchor {
		return index.NormalizePath(defFile) == headingFile
	}
	resolved := index.ResolveRelTarget(defFile, t.path)
	return resolved == headingFile
}

// refDefDestTarget is the parsed shape of a ref-def URL.
// Kept private to this file — the index has its own
// `linkTarget` helper and we don't want to leak two slightly
// different parsers through one type.
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

// resolveURIAndSource returns the URI string and source bytes for
// a workspace-relative path. It scans open documents first and
// returns the client-provided URI verbatim when the file is held
// as a buffer; only when no open buffer matches does it fall back
// to the canonical workspaceURI + on-disk read.
//
// Without this, a rename's WorkspaceEdit could split same-file
// edits across two URI strings (e.g. the client's exact URI and
// the server's canonicalized form) — clients keying open buffers
// on the original URI would then apply only one side of the split
// and leave the buffer in a torn state.
func (s *Server) resolveURIAndSource(rel string) (string, []byte, bool) {
	rel = index.NormalizePath(rel)
	_, _, root := s.snapshotConfig()
	for _, openURI := range s.docs.openURIs() {
		// Combine the lookup and the path check into one
		// short-circuit so a concurrent didClose between
		// openURIs() and get() can't nil-deref doc, without
		// a separate uncoverable `if !found` branch.
		if doc, ok := s.docs.get(openURI); ok &&
			index.NormalizePath(workspaceRelative(root, doc.path)) == rel {
			return openURI, doc.text, true
		}
	}
	uri := s.workspaceURI(rel)
	if uri == "" {
		return "", nil, false
	}
	source, _, ok := s.docTextOrFile(uri)
	if !ok {
		return "", nil, false
	}
	return uri, source, true
}

// anchorEditForEdge converts one incoming-edge record into a
// concrete TextEdit on the source file. Returns false when the
// source file isn't readable (out of workspace, deleted) or when
// the edge's link can't be located in the source — the rename
// silently skips those rather than failing the whole request, since
// the alternative would block a user from renaming a heading because
// of an unrelated stale edge.
func (s *Server) anchorEditForEdge(e index.Edge, oldSlug, newSlug string) (string, textEdit, bool) {
	uri, source, ok := s.resolveURIAndSource(e.SourceFile)
	if !ok {
		// Edge points at a file the LSP can no longer read (closed
		// buffer + on-disk delete, or workspace boundary moved).
		// Skip rather than fail the whole rename.
		return "", textEdit{}, false
	}
	lines := splitLines(source)
	// SourceLine bounds check: the index can hold stale entries
	// after a closed-buffer edit or a watcher event we haven't
	// processed yet. Indexing past EOF would panic the rename.
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

// destBounds returns the byte offsets of the destination *content*
// — the byte just after `(` and the byte at the matching `)` — for
// a link starting at or after `from` on row. Callers slice
// row[open:close] to get the destination text. Nested parens are
// matched depth-aware, and backslash-escaped parens are treated as
// literal bytes (CommonMark allows `\(` / `\)` inside the
// destination), so a link like `[t](foo\(bar\)#sec)` resolves to
// the outer parens, not the escaped inner ones.
func destBounds(row []byte, from int) (int, int, bool) {
	open := -1
	for i := from; i < len(row)-1; i++ {
		// Skip backslash-escaped bytes so a literal `\]` inside the
		// text portion (CommonMark allows it) doesn't fool the
		// `](` lookahead into thinking the destination starts
		// earlier than it does.
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

// stableSortEdits sorts each file's TextEdit slice in reverse
// document order so a client that applies edits sequentially in
// array order ends up with the right buffer state. The LSP spec
// only forbids overlap; it does not pin down application order,
// and naive clients walk the array top-to-bottom. Emitting
// bottom-up means earlier (later-positioned) edits don't shift
// the offsets the next edit relies on — particularly when two
// edits share a line.
func stableSortEdits(changes map[string][]textEdit) {
	for uri, edits := range changes {
		sort.SliceStable(edits, func(i, j int) bool {
			a, b := edits[i].Range.Start, edits[j].Range.Start
			if a.Line != b.Line {
				return a.Line > b.Line
			}
			return a.Character > b.Character
		})
		changes[uri] = edits
	}
}

// renameLinkRef rewrites a link-reference def and every use of the
// label in the same file.
//
// Collision handling: a new label that matches another existing def
// in the file fails with InvalidParams — MDS053 / MDS054 would
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
	// linkRefEdits is guaranteed to produce at least one edit:
	// the cursor was tagged TokenRefDef or TokenRefUse, so a
	// matching def or use exists in the buffer.
	changes := map[string][]textEdit{p.TextDocument.URI: linkRefEdits(source, oldLabel, newName)}
	stableSortEdits(changes)
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: changes})
}

// labelConflict returns the conflicting label name (preserving the
// original casing) when newLabel matches a link-reference
// definition in the file other than the one being renamed. Empty
// string means "no conflict".
//
// The scan filters regex matches through goldmark's parser context
// (validRefDefMatches) so a `[label]: url`-looking line inside a
// fenced code block or PI body doesn't count as a real def — both
// shapes parse as content, not as references, and the rename must
// not flag a phantom collision against them.
func labelConflict(source []byte, oldLabel, newLabel string) string {
	body, _ := bodyAndFMOffset(source)
	for _, m := range validRefDefMatches(body) {
		if m.normLabel == oldLabel {
			// Defs that match the rename's source label are the
			// ones being rewritten — they don't count as collisions.
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
// matches inside code/PI blocks (which goldmark refuses as
// references) drop out before reaching the rename.
type validRefDefMatch struct {
	bodyLine  int    // 1-based line within body
	rawLabel  string // bytes between `[` and `]`, unmodified
	normLabel string // util.ToLinkReference normalized form
	matchIdx  []int  // refDefRegexpMatches indices for further use
}

// validRefDefMatches returns the ref-def regex matches that
// fall outside any AST node. Real reference definitions
// never appear in the AST (they're consumed into the parser
// context), so the inverse — "regex hit on a line goldmark
// didn't tuck into a block" — is the set of real defs. This
// drops paragraph-continuation lookalikes, code-block
// content, and PI bodies in one pass without needing a
// label-keyed wanted set.
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

// contentBlockLines returns the set of body-line numbers that
// goldmark consumed into any AST node. Real reference
// definitions never appear in the AST — they live in
// parser.Context.References — so any line covered by an AST
// node is by definition not a def. Walking every node
// (paragraphs, code blocks, headings, list items, blockquotes,
// HTML blocks, PIs) means the rename surface doesn't need to
// enumerate block types; it just trusts the AST.
//
// The root *ast.Document is skipped: its Lines() span the
// whole buffer, which would mask every line and break the
// "real def" detection entirely.
func contentBlockLines(root ast.Node, body []byte) map[int]bool {
	out := map[int]bool{}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Only block-level nodes have Lines(); calling it on
		// inline nodes panics. Skip the Document root too —
		// its Lines() span the whole buffer and would mask
		// every line — and LinkReferenceDefinition because
		// real defs ARE the lines we want regex hits to
		// survive on. Including them would defeat the
		// detection.
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

// validRefDefBodyLines reports body-line indices that hold a real
// reference definition (one goldmark accepted, not a code-block
// look-alike). prepareRenameAt uses it to suppress the rename UI
// on lines the parser doesn't treat as defs.
func validRefDefBodyLines(body []byte) map[int]bool {
	out := map[int]bool{}
	for _, m := range validRefDefMatches(body) {
		out[m.bodyLine] = true
	}
	return out
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
//
// Filtering goes through validRefDefMatches so a `[label]: url`-
// looking line inside a fenced code block or PI body does not get
// rewritten — those parse as content, and overwriting them would
// corrupt examples in the docs.
func refDefEditsInBody(
	body []byte, lines [][]byte, fmOffset int,
	oldLabel, newName string,
) []textEdit {
	var out []textEdit
	for _, m := range validRefDefMatches(body) {
		if m.normLabel != oldLabel {
			continue
		}
		fileLine := m.bodyLine + fmOffset
		if fileLine-1 >= len(lines) {
			continue
		}
		row := lines[fileLine-1]
		// refDefBracketBytes always finds the `[…]` here: the
		// regex already confirmed the def shape on this line,
		// and validRefDefMatches confirmed goldmark accepted it.
		bracket := refDefBracketBytes(row)
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
//
// Goldmark guarantees a parsed reference link has at least one
// text segment, so linkTextBounds always returns a valid offset
// pair — the textStart-from-AST → bodyLine → lineStart chain
// stays inside the table by construction.
func refUseEdit(
	l *ast.Link, body []byte, lines [][]byte, fmOffset int, newName string,
	bodyIdx bodyLineIndex,
) (textEdit, bool) {
	textStart, textEnd := linkTextBounds(l, body)
	labelStart, labelEnd, ok := labelBoundsInBody(body, textStart, textEnd, l.Reference.Type)
	if !ok {
		// Empty-text references (`[][id]`) and other shapes
		// linkTextBounds can't anchor leave us without a way
		// to locate the label in the source. Skip rather than
		// emit a corrupt edit.
		return textEdit{}, false
	}
	// labelStart/End are byte offsets in body, so the line
	// lookups stay inside the source's line table by
	// construction: lines is splitLines(source) which spans
	// front matter + body, and fmOffset is the same line shift
	// the body parse used.
	startLine := bodyIdx.LineOfOffset(labelStart) + fmOffset
	endLine := bodyIdx.LineOfOffset(labelEnd) + fmOffset
	startCol := labelStart - bodyIdx.LineStart(startLine-fmOffset)
	endCol := labelEnd - bodyIdx.LineStart(endLine-fmOffset)
	startCh := utf16FromByteOffset(lines[startLine-1], startCol)
	endCh := utf16FromByteOffset(lines[endLine-1], endCol)
	return textEdit{
		Range: Range{
			Start: Position{Line: startLine - 1, Character: startCh},
			End:   Position{Line: endLine - 1, Character: endCh},
		},
		NewText: newName,
	}, true
}

// labelBoundsInBody returns the body-byte offsets of the label
// inside a reference-style link, given the link's text-segment
// bounds and the goldmark Reference type. For full `[text][label]`
// the range covers the label content (between the inner `[` and
// `]`). For shortcut `[label]` and collapsed `[label][]` the
// range covers the text bracket's content (the text IS the
// label). Returns ok=false when the surrounding bracket structure
// doesn't match what the type implies — guarding against unusual
// goldmark output without dropping multi-line uses, where the
// label still sits between intact byte offsets even when the
// text spans newlines.
func labelBoundsInBody(body []byte, textStart, textEnd int, refType ast.ReferenceLinkType) (int, int, bool) {
	// Empty-text refs like `[][id]` or `![][id]` parse with no
	// text segments, so linkTextBounds returns (-1, -1). The
	// label still exists in the source but we can't locate it
	// without the text bracket as an anchor — skip the use to
	// avoid a body[-1] panic. The same trade-off lives in
	// internal/rules/noreferencestyle.
	if textStart < 0 || textEnd < 0 {
		return 0, 0, false
	}
	if refType == ast.ReferenceLinkFull {
		// `[text][label]`: the text closes at body[textEnd] with
		// `]`, then `[` opens the label.
		if textEnd >= len(body) || body[textEnd] != ']' {
			return 0, 0, false
		}
		if textEnd+1 >= len(body) || body[textEnd+1] != '[' {
			return 0, 0, false
		}
		labelOpen := textEnd + 2
		// Scan for the closing `]`, honoring backslash escapes.
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
	// Shortcut / collapsed: the label IS the text bracket.
	if textStart <= 0 || body[textStart-1] != '[' {
		return 0, 0, false
	}
	if textEnd >= len(body) || body[textEnd] != ']' {
		return 0, 0, false
	}
	return textStart, textEnd, true
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

// bracketPairs returns every top-level `[` / `]` pair on row, in
// left-to-right order. The walker is depth-aware: a `[` opens a new
// nesting level and a `]` closes the innermost open `[`, so a
// CommonMark link with balanced bracket text such as
// `[a [b]][label]` records two pairs — the outer text `[a [b]]` and
// the trailing `[label]` — instead of mis-pairing the inner `[b]`
// with the first `]`. Backslash-escaped brackets (`\[`, `\]`) and
// any backslash-escaped byte are skipped, so escapes never open or
// close a level.
type bracketPair struct{ open, close int }

func bracketPairs(row []byte) []bracketPair {
	var pairs []bracketPair
	var stack []int
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' && i+1 < len(row) {
			i++ // skip escaped byte
			continue
		}
		switch row[i] {
		case '[':
			stack = append(stack, i)
		case ']':
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				// Only emit pairs once we've popped back to the
				// outermost level. Inner balanced pairs are part
				// of the link's text content.
				pairs = append(pairs, bracketPair{open: top, close: i})
			}
		}
	}
	return pairs
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
