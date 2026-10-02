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
//
// The warm path is a done check that inlines into the caller; the
// cold path lives in getSlow because its defers would otherwise keep
// Get from inlining (see high-performance-go.md).
func (e *Entry) Get(build func() any) any {
	if e.done.Load() {
		return e.val
	}
	return e.getSlow(build)
}

// getSlow is Get's mutex-guarded cold path.
func (e *Entry) getSlow(build func() any) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.done.Load() {
		defer e.done.Store(true)
		e.val = build()
	}
	return e.val
}

// Map is a keyed set of Entry slots. It wraps a sync.Map so the
// "every value is an *Entry" invariant holds by construction: only
// Entry stores into it, and Range hands back typed keys. The zero Map
// is ready to use and must not be copied after first use.
type Map struct {
	m sync.Map // string -> *Entry
}

// Entry returns the *Entry stored under key, storing a new one on
// first use. It checks Load before LoadOrStore so the warm path never
// constructs the throwaway &Entry{} that LoadOrStore's second argument
// would otherwise allocate: Go evaluates that argument before
// LoadOrStore can report that the key already exists.
func (m *Map) Entry(key string) *Entry {
	if v, ok := m.m.Load(key); ok {
		return v.(*Entry)
	}
	v, _ := m.m.LoadOrStore(key, &Entry{})
	return v.(*Entry)
}

// Get runs build at most once for key and returns the cached value;
// it is Entry(key).Get(build).
func (m *Map) Get(key string, build func() any) any {
	return m.Entry(key).Get(build)
}

// Delete drops key's slot so the next Get runs build again. A build
// already in flight on the dropped Entry finishes against that Entry
// and is not visible to later lookups.
func (m *Map) Delete(key string) {
	m.m.Delete(key)
}

// Range calls f for each key, in no particular order, until f returns
// false. As with sync.Map.Range, f may call Delete.
func (m *Map) Range(f func(key string) bool) {
	m.m.Range(func(k, _ any) bool {
		return f(k.(string))
	})
}

// Clear drops every slot, deleting key by key with the same Range and
// Delete calls the rest of Map uses, so it needs no sync.Map method
// beyond those (the WASM build runs on tinygo's sync.Map).
func (m *Map) Clear() {
	m.m.Range(func(k, _ any) bool {
		m.m.Delete(k)
		return true
	})
}
