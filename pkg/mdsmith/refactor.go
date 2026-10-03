package mdsmith

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/jeduden/mdsmith/internal/index"
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
// change no byte (a heading or label renamed to its own text). The
// plan carries only edits: a symbol rename never moves a file.
func (s *Session) Rename(uri string, source []byte, as, oldName, newName string) (RefactorPlan, error) {
	kind, err := refactor.ParseRenameKind(as)
	if err != nil {
		return RefactorPlan{}, fmt.Errorf("rename: as must be \"heading\" or \"label\", got %q", as)
	}
	ws := s.buildRefactorWorkspace(uri, source)
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
	ws := s.buildRefactorWorkspace("", nil)
	p, err := refactor.Move(ws, src, dst)
	if err != nil {
		return RefactorPlan{}, err
	}
	return toRefactorPlan(p), nil
}

// renameError rewords refactor.Rename's sentinel outcomes into the
// engine API's error text; engine conflicts and NothingToRenameError
// (whose text already names the kind) pass through unchanged.
// Each case mirrors a CLI exit path (see cmd/mdsmith/rename.go).
func renameError(err error, uri, oldName string) error {
	var missing refactor.MissingSymbolError
	switch {
	case errors.Is(err, refactor.ErrAmbiguousRename):
		return fmt.Errorf(
			"%q matches both a heading and a link-ref label; pass as=\"heading\" or as=\"label\"", oldName)
	case errors.Is(err, refactor.ErrNoRenameTarget):
		return fmt.Errorf("no heading or link-ref label %q", oldName)
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
// Workspace seam: a transient index over every Markdown file in the
// workspace, read through the session's Workspace (with the edited
// buffer overlaid when a rename supplies one).
type sessionRefactorWorkspace struct {
	s             *Session
	idx           *index.Index
	overlayURI    string
	overlaySource []byte
}

func (w *sessionRefactorWorkspace) IncomingAnchorEdges(file, slug string) []index.Edge {
	return w.idx.IncomingEdges(file, slug)
}

func (w *sessionRefactorWorkspace) IncomingPathEdges(file string) []index.Edge {
	return w.idx.IncomingPathEdges(file)
}

func (w *sessionRefactorWorkspace) IncomingWikilinkEdges(stem string) []index.Edge {
	return w.idx.IncomingWikilinkEdges(stem)
}

func (w *sessionRefactorWorkspace) Files() []string { return w.idx.Files() }

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

// buildRefactorWorkspace walks the session's workspace for Markdown
// files and builds a transient index over them. overlayURI, when set,
// substitutes overlaySource for that file's bytes so a rename computes
// against the caller's current buffer rather than the last-saved file.
func (s *Session) buildRefactorWorkspace(overlayURI string, overlaySource []byte) *sessionRefactorWorkspace {
	fsys := s.ws.FS()
	var rels []string
	// The walk callback swallows per-entry errors, so WalkDir's own return
	// is always nil for a well-formed workspace FS; nothing to propagate.
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if mdpath.HasMarkdownExt(path.Ext(p)) {
			rels = append(rels, index.NormalizePath(p))
		}
		return nil
	})
	overlayKey := index.NormalizePath(overlayURI)
	idx := index.New(s.rootDir)
	idx.BuildSerial(rels, func(rel string) ([]byte, error) {
		if overlayURI != "" && index.NormalizePath(rel) == overlayKey {
			return overlaySource, nil
		}
		return s.ws.ReadFile(rel)
	})
	return &sessionRefactorWorkspace{
		s:             s,
		idx:           idx,
		overlayURI:    overlayURI,
		overlaySource: overlaySource,
	}
}
