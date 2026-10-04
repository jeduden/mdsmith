package lsp

import (
	"encoding/json"
	"errors"
	"slices"
	"sync"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/refactor"
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
// refactor.ValidRefDefBodyLines lookup matches. The validation logic
// lives in internal/refactor so the LSP prepare-rename gate and the
// rename engine agree on what counts as a real def.
func isValidRefDefLine(source []byte, line int) bool {
	body, fmOffset := refactor.BodyAndFMOffset(source)
	bodyLine := line - fmOffset
	if bodyLine < 1 {
		return false
	}
	_, ok := refactor.ValidRefDefBodyLines(body)[bodyLine]
	return ok
}

// headingPrepareRange builds the rename range for an ATX or setext
// heading line. ATX headings have their leading and trailing `#`
// markers excluded; setext headings cover the full text line. The
// underline of a setext heading is left alone — CommonMark does not
// require its width to match the text, so the rename never touches
// it. The range comes from refactor.HeadingTextRange, the same
// function textDocument/rename uses to build its edit, so the popup
// never highlights a range the rename itself would not replace.
func headingPrepareRange(source []byte, line int, name string) (prepareRenameResult, bool) {
	lines := splitLines(source)
	if line-1 >= len(lines) {
		return prepareRenameResult{}, false
	}
	row := lines[line-1]
	startCol, endCol := refactor.HeadingTextRange(row)
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
	m := refactor.RefDefBracketBytes(row)
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
		if refactor.NormalizedLabel(row[pr.open+1:pr.close]) == label {
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
	if refactor.NormalizedLabel(row[next.open+1:next.close]) == label {
		return next.open + 1, next.close, true
	}
	if next.close == next.open+1 && refactor.NormalizedLabel(row[pr.open+1:pr.close]) == label {
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
	if pr.close == pr.open+1 && refactor.NormalizedLabel(row[prev.open+1:prev.close]) == label {
		return prev.open + 1, prev.close, true
	}
	if refactor.NormalizedLabel(row[pr.open+1:pr.close]) == label {
		return pr.open + 1, pr.close, true
	}
	return 0, 0, false
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
// edit computation lives in internal/refactor — this handler resolves
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

// lspRenameWorkspace backs the rename engine's heading seam
// (refactor.Workspace) with the server's warm index plus open buffers.
// The index supplies the edge graph; resolveURIAndSource supplies the
// per-file bytes and the URI the file's edits group under (the client
// URI for open buffers, the canonical workspace URI otherwise). A
// heading rename reads no wikilink index, so this type carries none;
// a move builds lspMoveWorkspace instead.
type lspRenameWorkspace struct {
	refactor.IndexEdges
	s *Server
}

// renameWorkspace returns the heading-rename Workspace over the warm
// index.
func (s *Server) renameWorkspace() lspRenameWorkspace {
	return lspRenameWorkspace{s: s, IndexEdges: refactor.NewIndexEdges(s.ensureIndex())}
}

// lspMoveWorkspace backs refactor.MoveWorkspace: the heading workspace
// plus the wikilink index a move reads. Build it only through
// moveWorkspace, which always sets wikilinks.
type lspMoveWorkspace struct {
	lspRenameWorkspace
	// wikilinks supplies the wikilink index a move reads, read once
	// per workspace for the root its paths were spelled against.
	wikilinks func() *linkgraph.WikilinkIndex
}

// moveWorkspace returns the move Workspace over the warm index, with a
// wikilink index read lazily, at most once, for root (see
// moveWikilinkIndex). Every move builds its workspace here, so none
// lacks the index a move's same-stem guard reads. A caller passes the
// root it spelled its paths against, so a config reload in between
// cannot key the index to another directory.
func (s *Server) moveWorkspace(root string) lspMoveWorkspace {
	return lspMoveWorkspace{
		lspRenameWorkspace: s.renameWorkspace(),
		wikilinks: sync.OnceValue(func() *linkgraph.WikilinkIndex {
			return s.moveWikilinkIndex(root)
		}),
	}
}

// moveWikilinkIndex returns the wikilink index a move at root reads.
// Once the server watches files, every create and delete drops the
// session's cached index, so that index is fresh and is read without
// a walk (the session walks only when nothing is cached yet). The
// session answers only for the root it was built at; a move spelled
// against another root (a config reload in between) walks that root.
// The client reports changes only under the workspace folder it
// watches, so a root outside that folder (mdsmith.config pointing
// elsewhere) walks too. Otherwise the cache may be stale, so root is
// walked fresh.
func (s *Server) moveWikilinkIndex(root string) *linkgraph.WikilinkIndex {
	if s.watchingFiles.Load() && s.watchesRoot(root) {
		sess, release := s.sessionAt(root)
		defer release()
		if sess != nil {
			return sess.WikilinkIndex()
		}
	}
	return s.walkWikilinks(root)
}

// watchesRoot reports whether root is the workspace folder the client
// watches or lies under it, so every file-set change below root reaches
// the server. Both paths are compared with symlinks resolved (see
// insideWorkspace): a root reached through a link out of the folder is
// not watched, since the client's watcher does not follow the link. A
// server with no workspace folder watches nothing.
func (s *Server) watchesRoot(root string) bool {
	s.configMu.RLock()
	folder := s.rootDir
	s.configMu.RUnlock()
	return root != "" && insideWorkspace(folder, root)
}

// WikilinkIndex implements refactor.MoveWorkspace: the index wikilink
// resolution reads (`[[stem]]` and typed `[[name.ext]]` alike), over the whole workspace root on disk, or nil when
// that root is unreadable.
func (w lspMoveWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex {
	return w.wikilinks()
}

func (w lspRenameWorkspace) Resolve(file string) (string, []byte, bool) {
	return w.s.resolveURIAndSource(file)
}

// renameHeading adapts refactor.Heading to the LSP wire: it delegates
// the slug-remap / anchor / ref-def-destination computation to the
// shared engine, then maps the neutral per-key Edit set to a
// WorkspaceEdit. A no-op rename yields an empty (but non-nil)
// Changes map, matching the pre-delegation behavior.
func (s *Server) renameHeading(
	msg *requestMessage, p renameParams,
	source []byte, rel string, line int, res index.LocateResult, newName string,
) {
	plan, err := refactor.Heading(s.renameWorkspace(), p.TextDocument.URI, rel, source, line, res.Name, newName)
	if err != nil {
		s.writeRenameError(msg.ID, err)
		return
	}
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: toLSPChanges(plan.Edits)})
}

// renameLinkRef adapts refactor.LinkRef to the LSP wire. The engine
// returns the def + use edits unordered; the handler sorts them
// bottom-up so a naive client applying them array-order leaves the
// buffer correct, exactly as the pre-delegation code did.
func (s *Server) renameLinkRef(
	msg *requestMessage, p renameParams,
	source []byte, oldLabel, newName string,
) {
	plan, err := refactor.LinkRef(p.TextDocument.URI, source, oldLabel, newName)
	if err != nil {
		s.writeRenameError(msg.ID, err)
		return
	}
	te := toTextEdits(plan.Edits[p.TextDocument.URI])
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
	var hce refactor.HeadingCollisionError
	if errors.As(err, &hce) {
		_ = s.t.writeErrorWithData(id, codeInvalidParams,
			hce.Error(), renameCollisionData{Conflict: hce.Conflict})
		return
	}
	var lce refactor.LabelConflictError
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
func toLSPChanges(changes map[string][]refactor.Edit) map[string][]textEdit {
	out := make(map[string][]textEdit, len(changes))
	for key, edits := range changes {
		out[key] = toTextEdits(edits)
	}
	return out
}

// toTextEdits copies neutral refactor.Edit values into LSP textEdits.
// refactor.Edit's Range is line + UTF-16 character, the same shape as
// the LSP textEdit, so the conversion is a field copy and the wire
// coordinates cannot drift.
func toTextEdits(edits []refactor.Edit) []textEdit {
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
// the array top-to-bottom. refactor.Heading already sorts its result
// this way internally; link-ref edits are sorted here so both paths
// emit the same bottom-up order — via the same comparator:
// refactor.ComparePositionsBottomUp, shared instead of duplicated. The
// conversion to refactor.Position is a zero-cost reinterpretation:
// both types have identical fields (Go ignores struct tags for
// convertibility), so this isn't a copy of anything but the two ints.
// If the two Position types ever diverge, this conversion stops
// compiling — a build failure here, not a silent runtime mismatch.
func sortTextEditsBottomUp(edits []textEdit) {
	slices.SortStableFunc(edits, func(a, b textEdit) int {
		return refactor.ComparePositionsBottomUp(
			refactor.Position(a.Range.Start), refactor.Position(b.Range.Start))
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
	if uri, doc, ok := s.docs.findByPath(func(path string) bool {
		return index.NormalizePath(workspaceRelative(root, path)) == rel
	}); ok {
		return uri, doc.text, true
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
