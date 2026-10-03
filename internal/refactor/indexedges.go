package refactor

import "github.com/jeduden/mdsmith/internal/index"

// IndexEdges implements the four index-backed Workspace methods
// (IncomingAnchorEdges, IncomingPathEdges, IncomingWikilinkEdges,
// Files) by forwarding to an index. A host embeds it and adds its own
// Resolve, so a new index-backed Workspace method is written once.
//
// Get returns the index on each call, so a host can build it lazily
// (calling Get on first use) or hand back a ready one via
// NewIndexEdges. A nil Get, or a Get that returns nil, answers every
// query with no edges and no files. Methods use value receivers so a
// host used by value still satisfies Workspace.
type IndexEdges struct {
	Get func() *index.Index
}

// NewIndexEdges returns an IndexEdges over an already-built index.
func NewIndexEdges(idx *index.Index) IndexEdges {
	return IndexEdges{Get: func() *index.Index { return idx }}
}

// index returns the wrapped index, or nil when none is configured;
// the index's nil-receiver queries then answer nothing.
func (e IndexEdges) index() *index.Index {
	if e.Get == nil {
		return nil
	}
	return e.Get()
}

// IncomingAnchorEdges implements Workspace.
func (e IndexEdges) IncomingAnchorEdges(file, slug string) []index.Edge {
	return e.index().IncomingEdges(file, slug)
}

// IncomingPathEdges implements Workspace.
func (e IndexEdges) IncomingPathEdges(file string) []index.Edge {
	return e.index().IncomingPathEdges(file)
}

// IncomingWikilinkEdges implements Workspace.
func (e IndexEdges) IncomingWikilinkEdges(stem string) []index.Edge {
	return e.index().IncomingWikilinkEdges(stem)
}

// Files implements Workspace.
func (e IndexEdges) Files() []string { return e.index().Files() }
