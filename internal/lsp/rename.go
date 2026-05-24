package lsp

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rename"
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
// real reference definition (one goldmark accepted), translating the
// source-coordinate line into body coordinates so the
// rename.ValidRefDefBodyLines lookup matches. The validation logic
// lives in internal/rename so the LSP prepare-rename gate and the
// rename engine agree on what counts as a real def.
func isValidRefDefLine(source []byte, line int) bool {
	body, fmOffset := rename.BodyAndFMOffset(source)
	bodyLine := line - fmOffset
	if bodyLine < 1 {
		return false
	}
	return rename.ValidRefDefBodyLines(body)[bodyLine]
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
	startCh := mdtext.UTF16FromByteOffset(row, startCol)
	endCh := mdtext.UTF16FromByteOffset(row, endCol)
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
	startCh := mdtext.UTF16FromByteOffset(row, m[0])
	endCh := mdtext.UTF16FromByteOffset(row, m[1])
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
	// After `]` we need `:` to qualify as a definition.
	if closeIdx+1 >= len(row) || row[closeIdx+1] != ':' {
		return nil
	}
	return []int{open, closeIdx}
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
	startCh := mdtext.UTF16FromByteOffset(row, startByte)
	endCh := mdtext.UTF16FromByteOffset(row, endByte)
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

// handleRename answers textDocument/rename. The reply is a
// WorkspaceEdit that covers every affected file. Heading rename
// rewrites incoming anchor links across the workspace; link-ref
// label rename rewrites the def and every use in the same file. The
// edit computation lives in internal/rename — this handler resolves
// the cursor, delegates, and adapts the neutral edits / typed errors
// to LSP wire types. Collisions return InvalidParams with
// renameCollisionData so the client can show a meaningful error
// instead of partially applying an edit.
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

// lspRenameWorkspace backs the rename engine's Workspace seam with
// the server's warm index plus open buffers. The index supplies the
// edge graph; resolveURIAndSource supplies the per-file bytes and
// the URI the file's edits group under (the client URI for open
// buffers, the canonical workspace URI otherwise).
type lspRenameWorkspace struct {
	s   *Server
	idx *index.Index
}

// Trivial index pass-through; no dedicated test by design (the
// rename engine's behavioral tests exercise it through Heading).
func (w lspRenameWorkspace) IncomingAnchorEdges(file, slug string) []index.Edge {
	return w.idx.IncomingEdges(file, slug)
}

// Trivial index pass-through; no dedicated test by design.
func (w lspRenameWorkspace) Files() []string { return w.idx.Files() }

// Trivial pass-through to resolveURIAndSource; no dedicated test by
// design (covered via resolveURIAndSource's own tests and the
// rename behavioral suite).
func (w lspRenameWorkspace) Resolve(file string) (string, []byte, bool) {
	return w.s.resolveURIAndSource(file)
}

// renameHeading adapts rename.Heading to the LSP wire: it delegates
// the slug-remap / anchor / ref-def-destination computation to the
// shared engine, then maps the neutral per-key Edit set to a
// WorkspaceEdit. A no-op rename yields an empty (but non-nil)
// Changes map, matching the pre-delegation behavior.
func (s *Server) renameHeading(
	msg *requestMessage, p renameParams,
	source []byte, rel string, line int, res index.LocateResult, newName string,
) {
	ws := lspRenameWorkspace{s: s, idx: s.ensureIndex()}
	changes, err := rename.Heading(ws, p.TextDocument.URI, rel, source, line, res.Name, newName)
	if err != nil {
		s.writeRenameError(msg.ID, err)
		return
	}
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: toLSPChanges(changes)})
}

// renameLinkRef adapts rename.LinkRef to the LSP wire. The engine
// returns the def + use edits unordered; the handler sorts them
// bottom-up so a naive client applying them array-order leaves the
// buffer correct, exactly as the pre-delegation code did.
func (s *Server) renameLinkRef(
	msg *requestMessage, p renameParams,
	source []byte, oldLabel, newName string,
) {
	edits, err := rename.LinkRef(source, oldLabel, newName)
	if err != nil {
		s.writeRenameError(msg.ID, err)
		return
	}
	te := toTextEdits(edits)
	sortTextEditsBottomUp(te)
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{
		Changes: map[string][]textEdit{p.TextDocument.URI: te},
	})
}

// writeRenameError maps a rename engine error to an LSP error
// response. Collision errors carry the conflicting name in
// renameCollisionData so the client can render it; every other
// typed error (empty / control rune / invalid label rune / empty
// slug) surfaces its message verbatim — the engine's Error() text
// is the same string the handler emitted before delegation.
func (s *Server) writeRenameError(id json.RawMessage, err error) {
	var hce rename.HeadingCollisionError
	if errors.As(err, &hce) {
		_ = s.t.writeErrorWithData(id, codeInvalidParams,
			hce.Error(), renameCollisionData{Conflict: hce.Conflict})
		return
	}
	var lce rename.LabelConflictError
	if errors.As(err, &lce) {
		_ = s.t.writeErrorWithData(id, codeInvalidParams,
			lce.Error(), renameCollisionData{Conflict: lce.Conflict})
		return
	}
	_ = s.t.writeError(id, codeInvalidParams, err.Error())
}

// toLSPChanges converts the engine's per-key Edit map to the LSP
// WorkspaceEdit shape. The returned map is always non-nil; with the
// omitempty tag on Changes, both nil and empty maps are omitted on the
// wire, so a no-op rename produces {} on the wire.
func toLSPChanges(changes map[string][]rename.Edit) map[string][]textEdit {
	out := make(map[string][]textEdit, len(changes))
	for key, edits := range changes {
		out[key] = toTextEdits(edits)
	}
	return out
}

// toTextEdits copies neutral rename.Edit values into LSP textEdits.
// rename.Edit's Range is line + UTF-16 character, the same shape as
// the LSP textEdit, so the conversion is a field copy and the wire
// coordinates cannot drift.
func toTextEdits(edits []rename.Edit) []textEdit {
	out := make([]textEdit, len(edits))
	for i, e := range edits {
		out[i] = textEdit{
			Range: Range{
				Start: Position{Line: e.Range.Start.Line, Character: e.Range.Start.Character},
				End:   Position{Line: e.Range.End.Line, Character: e.Range.End.Character},
			},
			NewText: e.NewText,
		}
	}
	return out
}

// sortTextEditsBottomUp orders edits in reverse document order so a
// client applying them sequentially in array order doesn't shift
// the offsets a later edit relies on. The LSP spec only forbids
// overlap; it doesn't pin application order, and naive clients walk
// the array top-to-bottom. rename.Heading already sorts its result
// this way internally; link-ref edits are sorted here so both paths
// emit the same bottom-up order.
func sortTextEditsBottomUp(edits []textEdit) {
	sort.SliceStable(edits, func(i, j int) bool {
		a, b := edits[i].Range.Start, edits[j].Range.Start
		if a.Line != b.Line {
			return a.Line > b.Line
		}
		return a.Character > b.Character
	})
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
