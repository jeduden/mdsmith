package lsp

import (
	"sync"
	"sync/atomic"

	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// sessionLease counts the callers holding one session, so a rebuild can
// release a superseded session's OS handles without closing them under
// a lint still using it. A rebuild retires the old lease; the session is
// disposed (closing the root it lends its runners) and its overlay
// closed (closing the overlay's disk root) once it is retired and its
// last holder releases it, whichever happens second.
type sessionLease struct {
	sess    *mdsmith.Session
	ws      *mdsmith.OverlayWorkspace
	holders atomic.Int64
	retired atomic.Bool
	once    sync.Once
}

// acquire counts one more holder and returns its release. The caller
// takes the lease while holding sessionMu, so a rebuild, which swaps the
// lease under the write lock, retires it only after every acquire on it
// has counted. Calling release more than once is a no-op.
func (l *sessionLease) acquire() (release func()) {
	l.holders.Add(1)
	var released atomic.Bool
	return func() {
		if released.Swap(true) {
			return
		}
		if l.holders.Add(-1) == 0 && l.retired.Load() {
			l.dispose()
		}
	}
}

// retire marks the lease superseded and disposes the session at once
// when no caller holds it. With both atomics sequentially consistent,
// a release dropping the count to zero and this retire cannot both miss
// the other; once guards the case where both see it.
func (l *sessionLease) retire() {
	l.retired.Store(true)
	if l.holders.Load() == 0 {
		l.dispose()
	}
}

// dispose releases the session's handles exactly once.
func (l *sessionLease) dispose() {
	l.once.Do(func() {
		l.sess.Dispose()
		l.ws.Close()
	})
}

// noRelease is the release handed out with no session.
func noRelease() {}
