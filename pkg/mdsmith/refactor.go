package mdsmith

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sync"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/refactor"
)

// TextEdit is one single-file text replacement in the engine's
// LSP-style coordinates: zero-based line numbers and UTF-16 character
// offsets, a half-open [start, end) range. The host applies it to the
// file its RefactorPlan key names.
type TextEdit struct {
	StartLine int    `json:"startLine"`
	StartChar int    `json:"startChar"`
	EndLine   int    `json:"endLine"`
	EndChar   int    `json:"endChar"`
	NewText   string `json:"newText"`
}

// FileMove describes a file relocation a RefactorPlan asks the host to
// perform, both paths workspace-relative. It is set only for a move,
// nil for a rename. The engine never performs it — a WASM host renames
// through its own platform API (the vault, the editor), since git mv is
// unavailable under wasm.
type FileMove struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// RefactorPlan is the engine's neutral result of a rename or move: text
// edits grouped by the workspace-relative file (the map key) they apply
// to, plus an optional file move. The host applies the edits and runs
// any move; the engine touches no files. It mirrors the internal
// refactor.Plan across the public Go and JavaScript surfaces.
type RefactorPlan struct {
	Edits map[string][]TextEdit `json:"edits"`
	Move  *FileMove             `json:"move,omitempty"`
}

// Rename computes a RefactorPlan that renames a heading or a
// link-reference label in the file at uri (whose current bytes are
// source) and rewrites every dependent reference across the workspace.
// as selects the kind — "heading" or "label" — or "" to auto-detect
// from source (a heading whose visible text is oldName, or a label
// normalizing to oldName; ambiguous or absent is an error). It also
// errors, rather than returning an empty plan, when an explicit kind
// finds no heading or label named oldName, or when the rename would
// have no effect (a heading or label renamed to its own text, or a
// heading renamed to the visible text it already renders as); that
// last error matches ErrNothingToRename, so a host can treat it as the
// harmless no-op it is. The plan carries only edits: a symbol rename
// never moves a file.
func (s *Session) Rename(uri string, source []byte, as, oldName, newName string) (RefactorPlan, error) {
	kind, err := refactor.ParseRenameKind(as)
	if err != nil {
		return RefactorPlan{}, fmt.Errorf("rename: as must be %s, got %q",
			refactor.RenameKindList("%q"), as)
	}
	// The workspace indexes lazily: only a heading rename queries
	// incoming edges, so a label rename or a failed detection never
	// walks a large WASM vault.
	ws := s.buildRefactorWorkspace(uri, source, isMarkdownPath)
	key := index.NormalizePath(uri)
	p, err := refactor.Rename(ws, key, source, kind, oldName, newName)
	if err != nil {
		return RefactorPlan{}, renameError(err, uri, oldName)
	}
	return toRefactorPlan(p), nil
}

// Move computes a RefactorPlan that relocates the workspace file src to
// dst and rewrites every reference, including the moved file's own
// outbound relative links. The plan's Move field names the relocation
// the host performs; the engine writes nothing. A traversal path, an
// equal src/dst, a missing source, or an existing destination is
// returned as an error.
func (s *Session) Move(src, dst string) (RefactorPlan, error) {
	ws := s.buildRefactorWorkspace("", nil, isMovePath)
	p, err := refactor.Move(ws, src, dst)
	if err != nil {
		return RefactorPlan{}, err
	}
	return toRefactorPlan(p), nil
}

// renameError rewords refactor.Rename's sentinel outcomes into the
// engine API's error text. Engine conflicts and a NothingToRenameError
// (whose text already names the kind, and which matches
// ErrNothingToRename) pass through unchanged.
// Each case mirrors a CLI exit path (see cmd/mdsmith/rename.go).
func renameError(err error, uri, oldName string) error {
	var missing refactor.MissingSymbolError
	switch {
	case errors.Is(err, refactor.ErrAmbiguousRename):
		return fmt.Errorf(
			"%q matches both %s; pass %s",
			oldName, refactor.RenameSymbolList("and", true), refactor.RenameKindList("as=%q"))
	case errors.Is(err, refactor.ErrNoRenameTarget):
		return fmt.Errorf("no %s %q", refactor.RenameSymbolList("or", false), oldName)
	case errors.As(err, &missing):
		return fmt.Errorf("%w in %s", missing, uri)
	}
	return err
}

// toRefactorPlan converts the internal refactor.Plan to the public
// RefactorPlan the Go and JS surfaces expose.
func toRefactorPlan(p refactor.Plan) RefactorPlan {
	out := RefactorPlan{Edits: make(map[string][]TextEdit, len(p.Edits))}
	for k, edits := range p.Edits {
		te := make([]TextEdit, len(edits))
		for i, e := range edits {
			te[i] = TextEdit{
				StartLine: e.Range.Start.Line,
				StartChar: e.Range.Start.Character,
				EndLine:   e.Range.End.Line,
				EndChar:   e.Range.End.Character,
				NewText:   e.NewText,
			}
		}
		out.Edits[k] = te
	}
	if p.FileOp != nil {
		out.Move = &FileMove{From: p.FileOp.From, To: p.FileOp.To}
	}
	return out
}

// sessionRefactorWorkspace adapts a Session to the refactor engine's
// MoveWorkspace seam: a transient index over every Markdown file in the
// workspace, read through the session's Workspace (with the edited
// buffer overlaid when a rename supplies one).
type sessionRefactorWorkspace struct {
	refactor.IndexEdges
	s *Session
	// paths lists the workspace FS's files the walk keeps, walked once
	// on first use and shared by the edge index (its Markdown files)
	// and, for a move, WikilinkIndex (those outside `.git` and
	// `node_modules`), so a move walks the FS once (see
	// walkWorkspacePaths).
	paths         func() []string
	overlayURI    string
	overlaySource []byte
}

// WikilinkIndex implements refactor.MoveWorkspace: the index wikilink
// resolution reads (`[[stem]]` and typed `[[name.ext]]` alike), over every file in the session workspace's tree
// except `.git` and `node_modules`. It is built from the path list the
// edge-index walk collects, so it walks nothing itself.
func (w *sessionRefactorWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex {
	return linkgraph.NewWikilinkIndexFromPaths(w.paths())
}

func (w *sessionRefactorWorkspace) Resolve(file string) (string, []byte, bool) {
	rel := index.NormalizePath(file)
	if w.overlayURI != "" && rel == index.NormalizePath(w.overlayURI) {
		return rel, w.overlaySource, true
	}
	src, err := w.s.ws.ReadFile(rel)
	if err != nil {
		return "", nil, false
	}
	return rel, src, true
}

// buildRefactorWorkspace returns a workspace whose edge and Files
// queries build a transient index over the session workspace's Markdown
// files on the first such query, reusing it after that; Resolve alone
// reads only the file it names, so a label rename or a failed detection
// never walks a large WASM vault. One walk of one FS snapshot (a
// MemWorkspace copies every file's bytes per FS call), taken through the
// session's source view so an OSWorkspace walks the root the session
// already holds, lists every file
// for both the edge index and WikilinkIndex; keep picks which paths the
// walk holds (isMarkdownPath for a symbol rename, which never builds a
// wikilink index, isMovePath for a move). overlayURI, when set,
// substitutes overlaySource for that file's bytes so a rename computes
// against the caller's current buffer rather than the last-saved file.
func (s *Session) buildRefactorWorkspace(
	overlayURI string, overlaySource []byte, keep func(string) bool,
) *sessionRefactorWorkspace {
	paths := sync.OnceValue(func() []string {
		src := s.sourceFS()
		defer src.release()
		return walkWorkspacePaths(src.FS, keep)
	})
	return &sessionRefactorWorkspace{
		IndexEdges: refactor.NewLazyIndexEdges(func() *index.Index {
			return s.indexRefactorWorkspace(paths(), overlayURI, overlaySource)
		}),
		s:             s,
		paths:         paths,
		overlayURI:    overlayURI,
		overlaySource: overlaySource,
	}
}

// walkWorkspacePaths walks fsys once and returns the file paths keep
// accepts. The walk callback swallows per-entry errors, so an unreadable
// root or subtree just contributes no paths. fsys is the session's
// source view (sourceFS): an OSWorkspace's is the root the session
// already lends its Checks, so the walk opens no handle of its own.
func walkWorkspacePaths(fsys fs.FS, keep func(string) bool) []string {
	var paths []string
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && keep(p) {
			paths = append(paths, p)
		}
		return nil
	})
	return paths
}

// isMarkdownPath keeps the files a symbol rename's edge index reads:
// every Markdown file, at any depth.
func isMarkdownPath(p string) bool {
	return mdpath.HasMarkdownExt(path.Ext(p))
}

// isMovePath keeps the files a move reads: every Markdown file (the
// edge index) and every file WikilinkIndex keys, which leaves out
// `.git` and `node_modules`. A non-Markdown file under those
// directories is read by neither, so a large `node_modules` adds no
// held paths.
func isMovePath(p string) bool {
	return isMarkdownPath(p) || linkgraph.WikilinkIndexed(p)
}

// ownsFS reports whether a caller of ws.FS owns the view it gets, and so
// closes it. An OSWorkspace opens a fresh os.Root per call that nothing
// else holds. Any other workspace — the LSP's overlay, which shares one
// cached disk root, or a host's own — may hand out an FS it keeps, so a
// caller leaves that one open.
func ownsFS(ws Workspace) bool {
	switch ws.(type) {
	case OSWorkspace, *OSWorkspace:
		return true
	}
	return false
}

// indexRefactorWorkspace indexes the Markdown files among paths, the
// session workspace's walked file list, reading overlaySource in place
// of overlayURI's bytes when overlayURI is set.
func (s *Session) indexRefactorWorkspace(paths []string, overlayURI string, overlaySource []byte) *index.Index {
	var rels []string
	for _, p := range paths {
		if isMarkdownPath(p) {
			rels = append(rels, index.NormalizePath(p))
		}
	}
	overlayKey := index.NormalizePath(overlayURI)
	idx := index.New(s.rootDir)
	idx.BuildSerial(rels, func(rel string) ([]byte, error) {
		if overlayURI != "" && index.NormalizePath(rel) == overlayKey {
			return overlaySource, nil
		}
		return s.ws.ReadFile(rel)
	})
	return idx
}
