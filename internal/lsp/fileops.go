package lsp

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"

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

// handleWillRenameFiles answers workspace/willRenameFiles. It plans
// every file the client is about to rename as one batch with
// refactor.MoveAll against the warm index and open buffers, returning
// the WorkspaceEdit that rewrites incoming references, ref-def
// destinations, wikilink stems, and each moved file's own outbound
// links. The client applies the edit, then performs the renames itself
// — so the reply carries no file operation, only text edits.
//
// The batch reads every link against every move: a link from one
// moved file to another is spelled from the holder's new folder to the
// target's new path, once. A file whose move cannot be planned (a
// destination that already exists, a traversal path) contributes no
// edit rather than failing the whole request: the editor still
// performs the rename, and any stranded link surfaces as an MDS027
// diagnostic. A rename pair listed twice is planned once.
//
// dropConflictingTextEdits and dropCrossMoveEdits stay as guards: the
// batch plans one edit per range and no path rewrite inside a moved
// file it did not plan for that file's new location, so neither drops
// anything. A window/logMessage warning counts every link the batch
// left stale (refactor.BatchPlan.Withheld) plus any edit a guard drops.
func (s *Server) handleWillRenameFiles(msg *requestMessage) {
	var p renameFilesParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid willRenameFiles params")
		return
	}
	_, _, root := s.snapshotConfig()
	// The batch shares one wikilink index, walked at the root its move
	// paths are spelled against.
	ws := s.renameWorkspace(root)

	batch := planRenameBatch(ws, root, p.Files)
	merged, dropped := guardRenameEdits(batch)
	if withheld := batch.withheld + dropped; withheld > 0 {
		warn := fmt.Sprintf("mdsmith: withheld %d link rewrite(s) between files renamed together; "+
			"re-check those links (MDS027 flags any that no longer resolve)", withheld)
		s.logger.Printf("%s", warn)
		_ = s.t.writeNotification("window/logMessage", logMessageParams{
			Type: messageTypeWarning, Message: warn,
		})
	}
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: merged})
}

// guardRenameEdits converts the batch's edits to LSP text edits and
// runs both guards over them, returning the kept edits and how many
// the guards dropped.
func guardRenameEdits(batch renameBatch) (map[string][]textEdit, int) {
	merged := map[string][]textEdit{}
	total, kept := 0, 0
	for key, edits := range batch.edits {
		merged[key] = toTextEdits(edits)
		total += len(edits)
	}
	for key, edits := range merged {
		edits = dropConflictingTextEdits(edits)
		if len(edits) == 0 {
			delete(merged, key)
			continue
		}
		merged[key] = edits
	}
	dropCrossMoveEdits(merged, batch.moves, batch.stems)
	for _, edits := range merged {
		kept += len(edits)
	}
	return merged, total - kept
}

// planRenameBatch runs refactor.MoveAll over the renames in files,
// read against root. A pair with an empty or unchanged path is
// skipped, and a pair listed twice is planned once. Every move whose
// source is readable is recorded in request order, planned or not,
// because the editor moves it anyway; a planned move keeps the edits
// the batch planned inside the moved file, for dropCrossMoveEdits.
func planRenameBatch(ws refactor.Workspace, root string, files []fileRename) renameBatch {
	var pairs []refactor.MovePair
	seen := map[refactor.MovePair]bool{}
	for _, f := range files {
		pair := refactor.MovePair{
			Src: index.NormalizePath(workspaceRelative(root, uriToPath(f.OldURI))),
			Dst: index.NormalizePath(workspaceRelative(root, uriToPath(f.NewURI))),
		}
		if pair.Src == "" || pair.Dst == "" || pair.Src == pair.Dst || seen[pair] {
			continue
		}
		seen[pair] = true
		pairs = append(pairs, pair)
	}
	bp := refactor.MoveAll(ws, pairs)
	batch := renameBatch{edits: bp.Edits, stems: bp.StemEdits, withheld: bp.Withheld}
	for _, m := range bp.Moves {
		if m.Key == "" {
			continue
		}
		pm := plannedMove{key: m.Key, changesDir: path.Dir(m.Src) != path.Dir(m.Dst)}
		if m.Err == nil {
			pm.own = bp.Edits[m.Key]
		}
		batch.moves = append(batch.moves, pm)
	}
	return batch
}

// renameBatch is one willRenameFiles request after planning: the
// batch's edits and their `[[stem]]` subset, keyed by edit key, every
// moved file, and the count of links the batch left stale
// (refactor.BatchPlan.Withheld).
type renameBatch struct {
	moves    []plannedMove
	edits    map[string][]refactor.Edit
	stems    map[string][]refactor.Edit
	withheld int
}

// plannedMove is one moved file of a willRenameFiles batch: key is its
// edit key (its URI as ws.Resolve returns it), changesDir reports
// whether the move lands in another directory, and own holds the edits
// the batch planned inside the file for its new location — nil when
// its move could not be planned.
type plannedMove struct {
	key        string
	changesDir bool
	own        []refactor.Edit
}

// dropCrossMoveEdits withholds from merged every path rewrite inside a
// file moved to a new directory that the batch did not plan for that
// file's new location. Such a rewrite would be spelled from the file's
// old directory, so its text would be wrong once the file lands
// elsewhere. A `[[stem]]` rewrite (from stems) is kept: it names its
// target by stem, which no directory change affects. A rename within
// one directory keeps every edit: the spelling base does not change.
// refactor.MoveAll plans no such rewrite, so this is a guard: a path
// edit inside a file whose move could not be planned is the one it
// would drop. Keys left with no edit are deleted.
func dropCrossMoveEdits(merged map[string][]textEdit, moves []plannedMove, stems map[string][]refactor.Edit) {
	for _, m := range moves {
		if !m.changesDir {
			continue
		}
		edits, ok := merged[m.key]
		if !ok {
			continue
		}
		keep := map[textEdit]bool{}
		for _, e := range toTextEdits(m.own) {
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
// another edit and returns the rest sorted bottom-up. LSP clients
// reject a WorkspaceEdit with overlapping ranges, which would lose
// every rewrite in the batch, and neither of two overlapping rewrites
// is known to be right, so both are dropped and any link left stale
// surfaces as an MDS027 diagnostic. refactor.MoveAll plans one edit per
// range, so this is a guard that keeps the reply valid should a plan
// ever overlap.
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
