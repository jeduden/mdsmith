// Package memo provides the build-once cache slot shared by
// internal/lint's per-File Memo and internal/runcache's whole-run
// Cache. It answers one question: how does a keyed cache run an
// expensive build exactly once per key, under concurrent readers,
// without allocating on the warm path? It imports only the standard
// library so both callers can depend on it without a cycle.
package memo

import (
	"sync"
	"sync/atomic"
)

// Entry guards a single cache slot so build runs exactly once even
// when several goroutines race for it. atomic.Bool + mutex is used
// instead of sync.Once because once.Do takes a function value: the
// closure `func() { e.val = build() }` a caller would pass captures e
// and build, so it allocates on every call, even when Do's internal
// check makes it a no-op. The atomic flag is a double-checked lock:
// one atomic load on the warm path, a mutex-guarded build on the cold
// path.
//
// The val/done/mu order keeps the pointer-bearing field first (see
// docs/development/high-performance-go.md "Struct layout").
//
// The zero Entry is ready to use.
type Entry struct {
	val  any
	done atomic.Bool
	mu   sync.Mutex
}

// Get runs build at most once for e, then returns the cached value.
// build is invoked directly, with no wrapping closure.
//
// Panic safety mirrors sync.Once: if build panics, the entry is still
// marked done (deferred Store) and the mutex is released (deferred
// Unlock), so the panic propagates without deadlocking the slot.
// Later calls return the zero value instead of re-running build.
func (e *Entry) Get(build func() any) any {
	if e.done.Load() {
		return e.val
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.done.Load() {
		defer e.done.Store(true)
		e.val = build()
	}
	return e.val
}

// GetWith is Get for a build that takes one argument. A caller whose
// build needs only arg can pass a package-level function value
// instead of a closure that captures arg, which avoids a per-call
// closure allocation. Panic safety matches Get.
func GetWith[T any](e *Entry, arg T, build func(T) any) any {
	if e.done.Load() {
		return e.val
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.done.Load() {
		defer e.done.Store(true)
		e.val = build(arg)
	}
	return e.val
}

// Load returns the *Entry stored under key in m, storing a new one on
// first use. Every value in m must be an *Entry. It checks Load before
// LoadOrStore so the warm path never constructs the throwaway
// &Entry{} that LoadOrStore's second argument would otherwise
// allocate: Go evaluates that argument before LoadOrStore can report
// that the key already exists.
func Load(m *sync.Map, key string) *Entry {
	if v, ok := m.Load(key); ok {
		return v.(*Entry)
	}
	v, _ := m.LoadOrStore(key, &Entry{})
	return v.(*Entry)
}
