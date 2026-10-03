package lsp

import (
	"encoding/json"
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
// planned once. Edits from two different moves that overlap (a link
// between two files renamed together) are withheld; see
// dropConflictingTextEdits.
func (s *Server) handleWillRenameFiles(msg *requestMessage) {
	var p renameFilesParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid willRenameFiles params")
		return
	}
	_, _, root := s.snapshotConfig()
	ws := lspRenameWorkspace{s: s, IndexEdges: refactor.NewIndexEdges(s.ensureIndex())}

	merged := map[string][]textEdit{}
	planned := map[[2]string]bool{}
	for _, f := range p.Files {
		src := index.NormalizePath(workspaceRelative(root, uriToPath(f.OldURI)))
		dst := index.NormalizePath(workspaceRelative(root, uriToPath(f.NewURI)))
		pair := [2]string{src, dst}
		if src == "" || dst == "" || src == dst || planned[pair] {
			continue
		}
		planned[pair] = true
		plan, err := refactor.Move(ws, src, dst)
		if err != nil {
			continue
		}
		for key, edits := range plan.Edits {
			merged[key] = append(merged[key], toTextEdits(edits)...)
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
	_ = s.t.writeResponse(msg.ID, &workspaceEdit{Changes: merged})
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
// two different moves. Batch-aware planning is tracked by plan
// 2610030438.
//
// Two edits overlap when their spans intersect or they share a start
// (a move never emits an insertion, so a shared start is a conflict);
// ranges that touch end-to-start do not. One sort and a sweep in
// document order find every overlapping edit: a run of edits each
// starting before the furthest end seen so far (or at the previous
// start) is a cluster in which every member overlaps another, so the
// whole cluster is dropped and a lone edit is kept. The slice is
// reused in place.
func dropConflictingTextEdits(edits []textEdit) []textEdit {
	if len(edits) < 2 {
		return edits
	}
	sortTextEditsBottomUp(edits)
	slices.Reverse(edits)
	n := 0
	for i := 0; i < len(edits); {
		end := edits[i].Range.End
		j := i + 1
		for ; j < len(edits); j++ {
			start := edits[j].Range.Start
			if start != edits[j-1].Range.Start && !posLess(start, end) {
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

// posLess reports whether a comes before b in the document. It reuses
// refactor.ComparePositionsBottomUp, which sorts later positions first,
// so a earlier than b compares greater.
func posLess(a, b Position) bool {
	return refactor.ComparePositionsBottomUp(refactor.Position(a), refactor.Position(b)) > 0
}
