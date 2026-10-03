package refactor

import "github.com/jeduden/mdsmith/internal/index"

// LazyWorkspace defers building a Workspace to its first method call
// and reuses that build afterwards. Rename consults its Workspace only
// for a heading rename (incoming anchors), so a host that wraps an
// expensive walk-and-index in a LazyWorkspace pays for it only when a
// heading rename needs it — a label rename or a failed detection reads
// no other file. Callers use it from one goroutine; it has no lock.
type LazyWorkspace struct {
	build func() Workspace
	ws    Workspace
}

// NewLazyWorkspace returns a Workspace that calls build on first use.
func NewLazyWorkspace(build func() Workspace) *LazyWorkspace {
	return &LazyWorkspace{build: build}
}

// get builds the workspace on first use and memoizes it.
func (l *LazyWorkspace) get() Workspace {
	if l.ws == nil {
		l.ws = l.build()
	}
	return l.ws
}

// IncomingAnchorEdges implements Workspace on the built workspace.
func (l *LazyWorkspace) IncomingAnchorEdges(file, slug string) []index.Edge {
	return l.get().IncomingAnchorEdges(file, slug)
}

// IncomingPathEdges implements Workspace on the built workspace.
func (l *LazyWorkspace) IncomingPathEdges(file string) []index.Edge {
	return l.get().IncomingPathEdges(file)
}

// IncomingWikilinkEdges implements Workspace on the built workspace.
func (l *LazyWorkspace) IncomingWikilinkEdges(stem string) []index.Edge {
	return l.get().IncomingWikilinkEdges(stem)
}

// Files implements Workspace on the built workspace.
func (l *LazyWorkspace) Files() []string { return l.get().Files() }

// Resolve implements Workspace on the built workspace.
func (l *LazyWorkspace) Resolve(file string) (string, []byte, bool) {
	return l.get().Resolve(file)
}
