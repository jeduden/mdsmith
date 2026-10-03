package lsp

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sync"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/refactor"
)

// markdownFileOperationCapabilities advertises that the server wants to
// be consulted on Markdown file renames, both before (willRename, so it
// can return the reference-rewriting edits the client applies) and
// after (didRename, so it can swap the path in the warm index). The
// filter limits the events to `.md` / `.markdown` files. Registration
// is inert unless the client also advertises
// workspace.fileOperations.willRename / didRename support.
func markdownFileOperationCapabilities() *workspaceServerCapabilities {
	filters := []fileOperationFilter{
		{Pattern: fileOperationPattern{Glob: "**/*.{md,markdown}", Matches: "file"}},
	}
	return &workspaceServerCapabilities{
		FileOperations: &fileOperationsServerCapabilities{
			WillRename: &fileOperationRegistrationOptions{Filters: filters},
			DidRename:  &fileOperationRegistrationOptions{Filters: filters},
		},
	}
}

// handleWillRenameFiles answers workspace/willRenameFiles. For every
// file the client is about to rename it runs refactor.Move against the
// warm index and open buffers, returning the merged WorkspaceEdit that
// rewrites incoming references, ref-def destinations, wikilink stems,
// and the moved file's own outbound links. The client applies the edit,
// then performs the rename itself — so the reply carries no file
// operation, only text edits.
//
// A file whose move cannot be planned (a destination that already
// exists, a traversal path) contributes no edit rather than failing the
// whole request: the editor still performs the rename, and any stranded
// link surfaces as an MDS027 diagnostic. A rename pair listed twice is
// planned once. Each move is planned against the pre-batch snapshot, so
// an edit that assumes another batch member stayed put is withheld:
// overlapping edits from two moves (see dropConflictingTextEdits) and
// any path rewrite one move plans inside another moved file whose
// directory changes (see dropCrossMoveEdits), including a batch
// member whose own move could not be planned; a `[[stem]]` rewrite
// does not depend on where its file sits, so it is kept. A
// window/logMessage warning names how many rewrites were withheld.
// Batch-aware planning is tracked by plan 2610030438.
func (s *Server) handleWillRenameFiles(msg *requestMessage) {
	var p renameFilesParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid willRenameFiles params")
		return
	}
	_, _, root := s.snapshotConfig()
	ws := lspRenameWorkspace{
		s: s, IndexEdges: refactor.NewIndexEdges(s.ensureIndex()),
		wikilinks: sync.OnceValue(s.buildWikilinkIndex),
	}

	moves := planRenameBatch(ws, root, p.Files)
	merged := map[string][]textEdit{}
	total, kept := 0, 0
	for _, m := range moves {
		for key, edits := range m.edits {
			merged[key] = append(merged[key], toTextEdits(edits)...)
			total += len(edits)
		}
	}
	for key, edits := range merged {
		edits = dropConflictingTextEdits(edits)
		if len(edits) == 0 {
			delete(merged, key)
			continue
		}
		merged[key] = edits
	}
	dropCrossMoveEdits(merged, moves)
	for _, edits := range merged {
		kept += len(edits)
	}
	if withheld := total - kept; withheld > 0 {
		warn := fmt.Sprintf("mdsmith: withheld %d link rewrite(s) between files renamed together; "+
			"re-check those links (MDS027 flags any that no longer resolve)", withheld)
		s.logger.Printf("%s", warn)
		_ = s.t.writeNotification("window/logMessage", logMessageParams{
			Type: messageTypeWarning, Message: warn,
		})
	}
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: merged})
}

// planRenameBatch runs refactor.Move for each rename in files, read
// against root, and returns the planned moves in request order. A pair
// with an empty or unchanged path is skipped, and a pair listed twice
// is planned once. A file whose move cannot be planned (an overwritten
// destination) is still recorded, with no edits, because the editor
// moves it anyway: dropCrossMoveEdits then withholds another move's
// path rewrite inside it. Each planned move also keeps the `[[stem]]`
// subset of its edits, which refactor.MoveWithStemEdits returns
// beside the plan, for dropCrossMoveEdits.
func planRenameBatch(ws refactor.Workspace, root string, files []fileRename) []plannedMove {
	var moves []plannedMove
	planned := map[[2]string]bool{}
	for _, f := range files {
		src := index.NormalizePath(workspaceRelative(root, uriToPath(f.OldURI)))
		dst := index.NormalizePath(workspaceRelative(root, uriToPath(f.NewURI)))
		pair := [2]string{src, dst}
		if src == "" || dst == "" || src == dst || planned[pair] {
			continue
		}
		planned[pair] = true
		key, _, ok := ws.Resolve(src)
		m := plannedMove{key: key, changesDir: path.Dir(src) != path.Dir(dst)}
		plan, stems, err := refactor.MoveWithStemEdits(ws, src, dst)
		if err != nil {
			if ok {
				moves = append(moves, m)
			}
			continue
		}
		m.edits, m.stemEdits = plan.Edits, stems
		moves = append(moves, m)
	}
	return moves
}

// plannedMove is one willRenameFiles move after planning: key is the
// moved file's edit key (its URI as ws.Resolve returns it), changesDir
// reports whether the move lands in another directory, edits is the
// plan's per-key edit set, and stemEdits is its `[[stem]]` subset
// (refactor.MoveWithStemEdits). A move that could not be planned has
// neither.
type plannedMove struct {
	key        string
	changesDir bool
	edits      map[string][]refactor.Edit
	stemEdits  map[string][]refactor.Edit
}

// dropCrossMoveEdits withholds from merged every path rewrite one move
// planned inside the file another move relocates to a new directory.
// That move spelled the rewritten path from the file's old directory,
// so the text is wrong once the file lands elsewhere — even when no
// other edit overlaps it (moving docs/b.md to docs/sub/b.md rewrites
// docs/a.md's `../docs/b.md` as `sub/b.md`, wrong after docs/a.md
// moves to other/). A `[[stem]]` rewrite is kept: it names its target
// by stem, which no directory change affects. A rename within one
// directory keeps every edit: the spelling base does not change.
//
// It runs after dropConflictingTextEdits, so a moved file's own
// rewrite of a link to a co-moved file is already gone with its
// overlapping partner, and every surviving edit is unique in its
// range. Keeping the edits the moved file's own move planned, plus any
// stem rewrite, therefore drops exactly the other moves' path
// rewrites, matched by value. Keys left with no edit are deleted.
func dropCrossMoveEdits(merged map[string][]textEdit, moves []plannedMove) {
	stems := map[string][]refactor.Edit{}
	for _, m := range moves {
		for key, edits := range m.stemEdits {
			stems[key] = append(stems[key], edits...)
		}
	}
	for _, m := range moves {
		if !m.changesDir {
			continue
		}
		edits, ok := merged[m.key]
		if !ok {
			continue
		}
		keep := map[textEdit]bool{}
		for _, e := range toTextEdits(m.edits[m.key]) {
			keep[e] = true
		}
		for _, e := range toTextEdits(stems[m.key]) {
			keep[e] = true
		}
		edits = slices.DeleteFunc(edits, func(e textEdit) bool { return !keep[e] })
		if len(edits) == 0 {
			delete(merged, m.key)
			continue
		}
		merged[m.key] = edits
	}
}

// handleDidRenameFiles processes the workspace/didRenameFiles
// notification: it swaps each renamed file's path in the warm index so
// later navigation and rename requests resolve against the new
// location. The client has already performed the rename and applied the
// willRename edits, so this only keeps the index consistent.
func (s *Server) handleDidRenameFiles(params json.RawMessage) {
	var p renameFilesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	_, _, root := s.snapshotConfig()
	idx := s.ensureIndex()
	for _, f := range p.Files {
		oldRel := index.NormalizePath(workspaceRelative(root, uriToPath(f.OldURI)))
		if oldRel != "" {
			idx.Remove(oldRel)
		}
		newPath := uriToPath(f.NewURI)
		newRel := index.NormalizePath(workspaceRelative(root, newPath))
		// insideWorkspace is the real containment guard: workspaceRelative
		// returns an out-of-root path unchanged (not ""), so newRel=="" does
		// not catch a destination outside the workspace, and
		// symbolWorkspace.ReadFile reads an absolute path off disk with no
		// boundary. Gate the read the same way the symbol path does — this
		// also refuses an in-workspace symlink that escapes the root.
		if newRel == "" || !isMarkdownExt(newPath) || !insideWorkspace(root, newPath) {
			continue
		}
		if data, err := symbolWorkspace.ReadFile(newPath); err == nil {
			idx.Update(newRel, data)
		}
	}
}

// dropConflictingTextEdits withholds every edit whose range overlaps
// another edit and returns the rest sorted bottom-up. A willRenameFiles
// batch plans each refactor.Move against the same pre-batch snapshot,
// so two moved files that link to each other get two rewrites of one
// link; LSP clients reject a WorkspaceEdit with overlapping ranges,
// which would lose every rewrite in the batch. Keeping either side
// would be wrong (each assumes the other file did not move), so both
// are dropped and any link left stale surfaces as an MDS027 diagnostic.
// That holds even when the two rewrites agree: docs/a.md's `../b.md`
// becomes `b.md` both when a.md moves to the root and when b.md moves
// into docs/, yet the right text is `docs/b.md`. handleWillRenameFiles
// plans a repeated rename pair once, so every overlap here comes from
// two different moves. A link only one move rewrites overlaps nothing
// here; dropCrossMoveEdits catches the case where that rewrite lands
// in another moved file.
//
// Two edits overlap when their spans intersect — an insert strictly
// inside a replacement included — or both are inserts at one position,
// whose relative order no single move fixes. Ranges that touch
// end-to-start do not overlap, and neither does an insert at a
// replacement's start or end, matching refactor.ApplyEdits and the LSP
// spec. One sort (by start, inserts first at a shared start) and a
// sweep in document order find every overlapping edit: a run of edits
// each starting before the furthest end seen so far (or an insert at
// the previous insert's position) is a cluster in which every member
// overlaps another, so the whole cluster is dropped and a lone edit is
// kept. The slice is reused in place.
func dropConflictingTextEdits(edits []textEdit) []textEdit {
	if len(edits) < 2 {
		return edits
	}
	slices.SortStableFunc(edits, compareTextEditsTopDown)
	n := 0
	for i := 0; i < len(edits); {
		end := edits[i].Range.End
		j := i + 1
		for ; j < len(edits); j++ {
			start := edits[j].Range.Start
			prev := edits[j-1]
			sameInsert := isInsert(edits[j]) && isInsert(prev) && start == prev.Range.Start
			if !sameInsert && !posLess(start, end) {
				break
			}
			if posLess(end, edits[j].Range.End) {
				end = edits[j].Range.End
			}
		}
		if j == i+1 {
			edits[n] = edits[i]
			n++
		}
		i = j
	}
	out := edits[:n]
	slices.Reverse(out)
	return out
}

// compareTextEditsTopDown orders edits by start in document order,
// putting an insert before a replacement that starts at the same
// position so the sweep in dropConflictingTextEdits sees the insert
// end where the replacement begins.
func compareTextEditsTopDown(a, b textEdit) int {
	if c := -refactor.ComparePositionsBottomUp(
		refactor.Position(a.Range.Start), refactor.Position(b.Range.Start)); c != 0 {
		return c
	}
	switch ai, bi := isInsert(a), isInsert(b); {
	case ai && !bi:
		return -1
	case bi && !ai:
		return 1
	}
	return 0
}

// isInsert reports whether e is a zero-width insertion.
func isInsert(e textEdit) bool {
	return e.Range.Start == e.Range.End
}

// posLess reports whether a comes before b in the document. It reuses
// refactor.ComparePositionsBottomUp, which sorts later positions first,
// so a earlier than b compares greater.
func posLess(a, b Position) bool {
	return refactor.ComparePositionsBottomUp(refactor.Position(a), refactor.Position(b)) > 0
}
