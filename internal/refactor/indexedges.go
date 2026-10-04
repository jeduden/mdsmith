package refactor

import (
	"sync"

	"github.com/jeduden/mdsmith/internal/index"
)

// IndexEdges implements the four index-backed Workspace and
// MoveWorkspace methods (IncomingAnchorEdges, IncomingPathEdges,
// IncomingWikilinkEdges, Files) by forwarding to an index. A host
// embeds it and adds its own Resolve (and, for a move, WikilinkIndex),
// so a new index-backed method is written once.
//
// The getter is called on every query; IndexEdges itself caches
// nothing. It is unexported, so a host builds an IndexEdges only
// through NewIndexEdges, which wraps a ready index, or
// NewLazyIndexEdges, which builds one on the first query and reuses
// it — never a raw builder that would re-index per query. The zero
// value, or a getter that returns nil, answers every query with no
// edges and no files. Methods use value receivers so a host used by
// value still satisfies the seam.
type IndexEdges struct {
	get func() *index.Index
}

// NewIndexEdges returns an IndexEdges over an already-built index.
func NewIndexEdges(idx *index.Index) IndexEdges {
	return IndexEdges{get: func() *index.Index { return idx }}
}

// NewLazyIndexEdges returns an IndexEdges that calls build on its
// first query and answers every later query, from any copy, from that
// same index. A host whose Resolve must not pay for indexing uses it.
// A nil build gives the zero IndexEdges, which answers nothing.
func NewLazyIndexEdges(build func() *index.Index) IndexEdges {
	if build == nil {
		return IndexEdges{}
	}
	return IndexEdges{get: sync.OnceValue(build)}
}

// index returns the wrapped index, or nil when none is configured;
// the index's nil-receiver queries then answer nothing.
func (e IndexEdges) index() *index.Index {
	if e.get == nil {
		return nil
	}
	return e.get()
}

// IncomingAnchorEdges implements Workspace.
func (e IndexEdges) IncomingAnchorEdges(file, slug string) []index.Edge {
	return e.index().IncomingEdges(file, slug)
}

// IncomingPathEdges implements MoveWorkspace.
func (e IndexEdges) IncomingPathEdges(file string) []index.Edge {
	return e.index().IncomingPathEdges(file)
}

// IncomingWikilinkEdges implements MoveWorkspace.
func (e IndexEdges) IncomingWikilinkEdges(stem string) []index.Edge {
	return e.index().IncomingWikilinkEdges(stem)
}

// Files implements Workspace.
func (e IndexEdges) Files() []string { return e.index().Files() }
