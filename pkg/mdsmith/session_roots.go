package mdsmith

import (
	"io/fs"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
)

// sessionRoots holds the disk handles a Session opens once and lends to
// every operation: a view of rootDir (the engine's RootFS) and, for a
// workspace whose FS view the caller owns (an OSWorkspace), that view.
// A File the session's parse cache keeps holds them past the call that
// parsed it, so only the session can close them, and Dispose does.
type sessionRoots struct {
	mu     sync.Mutex
	closed bool
	root   lint.RootFS
	source fs.FS
}

// lentRoot returns the view of s.rootDir every runner borrows as its
// RootFS, opening it on first use. It returns nil when the session has
// no on-disk root, or after Dispose (the runner then opens and closes
// a root of its own per call).
func (s *Session) lentRoot() fs.FS {
	if s.rootDir == "" {
		return nil
	}
	s.roots.mu.Lock()
	defer s.roots.mu.Unlock()
	if s.roots.closed {
		return nil
	}
	if s.roots.root == nil {
		s.roots.root = lint.OpenRootFS(s.rootDir)
	}
	return s.roots.root
}

// sourceFS returns the workspace FS view an operation reads through. A
// view the session owns (ownsFS) is opened once and reused; any other
// workspace hands out its own per call, as does an owned one after
// Dispose.
func (s *Session) sourceFS() fs.FS {
	if !ownsFS(s.ws) {
		return s.ws.FS()
	}
	s.roots.mu.Lock()
	defer s.roots.mu.Unlock()
	if s.roots.closed {
		return s.ws.FS()
	}
	if s.roots.source == nil {
		s.roots.source = s.ws.FS()
	}
	return s.roots.source
}

// closeRoots closes every handle the session opened. Later calls are
// no-ops.
func (s *Session) closeRoots() {
	s.roots.mu.Lock()
	defer s.roots.mu.Unlock()
	s.roots.closed = true
	for _, h := range []fs.FS{s.roots.root, s.roots.source} {
		if c, ok := h.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}
	s.roots.root, s.roots.source = nil, nil
}
