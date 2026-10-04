package lsp

import (
	"encoding/json"
	"fmt"
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
// dropConflictingTextEdits stays as a guard: the batch plans one edit
// per range, so it drops nothing, but an overlap would make the client
// reject the whole reply. A window/logMessage warning counts every
// link the batch left stale (refactor.BatchPlan.Withheld) plus any
// edit the guard drops.
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
	merged, dropped := guardRenameEdits(batch.Edits)
	if withheld := batch.Withheld + dropped; withheld > 0 {
		warn := fmt.Sprintf("mdsmith: withheld %d link rewrite(s) for files renamed together; "+
			"re-check those links (MDS027 flags any that no longer resolve)", withheld)
		s.logger.Printf("%s", warn)
		_ = s.t.writeNotification("window/logMessage", logMessageParams{
			Type: messageTypeWarning, Message: warn,
		})
	}
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: merged})
}

// guardRenameEdits converts a batch's edits, keyed by edit key, to LSP
// text edits and runs dropConflictingTextEdits over each file's,
// returning the kept edits and how many the guard dropped.
func guardRenameEdits(edits map[string][]refactor.Edit) (map[string][]textEdit, int) {
	merged := map[string][]textEdit{}
	dropped := 0
	for key, edits := range edits {
		kept := dropConflictingTextEdits(toTextEdits(edits))
		dropped += len(edits) - len(kept)
		if len(kept) > 0 {
			merged[key] = kept
		}
	}
	return merged, dropped
}

// planRenameBatch runs refactor.MoveAll over the renames in files,
// read against root. A pair with an empty or unchanged path is
// skipped, and a pair listed twice is planned once.
func planRenameBatch(ws refactor.Workspace, root string, files []fileRename) refactor.BatchPlan {
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
	return refactor.MoveAll(ws, pairs)
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
