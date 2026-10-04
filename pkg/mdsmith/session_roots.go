package mdsmith

import (
	"io/fs"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
)

// sessionRoots holds the disk handles a Session opens once and lends to
// every operation: a view of rootDir (the engine's RootFS) and, for a
// workspace whose FS view the session owns (an OSWorkspace), that view,
// which is the same handle when the workspace sits at rootDir.
// A File the session's parse cache keeps holds them past the call that
// parsed it, so only the session can close them, and Dispose does.
type sessionRoots struct {
	mu     sync.Mutex
	closed bool
	root   lint.RootFS
	source fs.FS
	// sourceIsRoot is set when source is root (an OSWorkspace at
	// rootDir), so closeRoots closes the one handle once.
	sourceIsRoot bool
}

// sourceView is the workspace FS view one operation reads through.
type sourceView struct {
	fs.FS
	// perCall is set when the view was opened for this operation alone
	// (after Dispose, or when the session could not open one to keep):
	// release closes it, and no parse cache may keep a File holding it.
	perCall bool
}

// release closes the view when it was opened for this operation alone;
// a view the session lends, or one its workspace keeps, stays open.
func (v sourceView) release() {
	if v.perCall {
		lint.CloseFS(v.FS)
	}
}

// lentRoot returns the view of s.rootDir every runner borrows as its
// RootFS, opening it on first use. It returns nil when the session has
// no on-disk root, after Dispose, or when the root cannot be opened
// (the next call retries the open rather than keeping the failure); the
// runner then opens and closes a root of its own per call.
func (s *Session) lentRoot() fs.FS {
	if s.rootDir == "" {
		return nil
	}
	s.roots.mu.Lock()
	defer s.roots.mu.Unlock()
	return s.lentRootLocked()
}

// lentRootLocked is lentRoot's body; the caller holds s.roots.mu and has
// checked that s.rootDir is set.
func (s *Session) lentRootLocked() fs.FS {
	if s.roots.closed {
		return nil
	}
	if s.roots.root == nil {
		root := lint.OpenRootFS(s.rootDir)
		if !readable(root) {
			lint.CloseFS(root)
			return nil
		}
		s.roots.root = root
	}
	return s.roots.root
}

// sourceFS returns the workspace FS view an operation reads through; the
// operation calls its release once it ends. A view the session owns
// (ownsFS) is opened once and reused until Dispose; an OSWorkspace's
// view is the lent root itself, since both are rooted at rootDir, so
// the session holds one handle rather than two. After Dispose, or
// when the open fails (retried on the next call), the operation gets a
// view of its own that release closes. Any other workspace hands out
// its own view per call and keeps what it holds, so release leaves it
// open.
func (s *Session) sourceFS() sourceView {
	if !ownsFS(s.ws) {
		return sourceView{FS: s.ws.FS()}
	}
	s.roots.mu.Lock()
	defer s.roots.mu.Unlock()
	if s.roots.source != nil {
		return sourceView{FS: s.roots.source}
	}
	if s.rootDir != "" {
		if root := s.lentRootLocked(); root != nil {
			s.roots.source, s.roots.sourceIsRoot = root, true
			return sourceView{FS: root}
		}
	}
	view := s.ws.FS()
	if s.roots.closed || !readable(view) {
		return sourceView{FS: view, perCall: true}
	}
	s.roots.source = view
	return sourceView{FS: view}
}

// readable reports whether fsys can read its own root, so a failed open
// (an unreadable or missing directory) is not kept for the session's
// lifetime.
func readable(fsys fs.FS) bool {
	_, err := fs.Stat(fsys, ".")
	return err == nil
}

// closeRoots closes every handle the session opened. Later calls are
// no-ops.
func (s *Session) closeRoots() {
	s.roots.mu.Lock()
	defer s.roots.mu.Unlock()
	s.roots.closed = true
	lint.CloseFS(s.roots.root)
	if !s.roots.sourceIsRoot {
		lint.CloseFS(s.roots.source)
	}
	s.roots.root, s.roots.source, s.roots.sourceIsRoot = nil, nil, false
}
