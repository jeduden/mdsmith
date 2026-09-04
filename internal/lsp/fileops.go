package lsp

import (
	"encoding/json"

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
// link surfaces as an MDS027 diagnostic.
func (s *Server) handleWillRenameFiles(msg *requestMessage) {
	var p renameFilesParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid willRenameFiles params")
		return
	}
	_, _, root := s.snapshotConfig()
	ws := lspRenameWorkspace{s: s, idx: s.ensureIndex()}

	merged := map[string][]textEdit{}
	for _, f := range p.Files {
		src := index.NormalizePath(workspaceRelative(root, uriToPath(f.OldURI)))
		dst := index.NormalizePath(workspaceRelative(root, uriToPath(f.NewURI)))
		if src == "" || dst == "" || src == dst {
			continue
		}
		plan, err := refactor.Move(ws, src, dst)
		if err != nil {
			continue
		}
		for key, edits := range plan.Edits {
			merged[key] = append(merged[key], toTextEdits(edits)...)
		}
	}
	for key := range merged {
		sortTextEditsBottomUp(merged[key])
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
