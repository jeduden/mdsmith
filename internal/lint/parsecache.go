package lint

import "sync"

// ParseCache memoizes the parsed *File for a single document keyed by
// the path the LSP hands to engine.Runner.RunSource (workspace-
// relative — produced by workspaceRelative(root, doc.path) on the
// server). Each entry remembers the textDocument/version the parse
// reflects; a lookup hits only when the caller's version matches the
// stored one, so an LSP edit that bumps the version forces the next
// runLint to reparse.
//
// One entry per path. Versions monotonically increase per LSP edit,
// so a stored older entry is dead the next time anyone looks. The
// cache lives for the server's lifetime; didChange / didClose /
// didChangeWatchedFiles call Invalidate to drop a path whose buffer
// is gone or whose on-disk content changed.
//
// Invalidate does not free the entry — it leaves a tombstone that
// records the next acceptable Put version. A late parse for the
// just-cleared version cannot re-insert a stale *File; only a Put at
// a strictly newer version takes the slot. The tombstone is one int
// and a nil pointer, replaced the next time a fresh parse lands.
//
// This cache is opt-in via engine.Runner.ParseCache — only the LSP
// installs one. Non-LSP callers (mdsmith check, embedded hosts) keep
// the cold parse path and pay nothing.
type ParseCache struct {
	mu      sync.Mutex
	entries map[string]parseCacheEntry
}

// parseCacheEntry pairs the cached *File with the LSP textDocument
// version it was parsed at. version is the discriminator on Get: a
// caller with a different version observes a miss and reparses.
// minPutVersion records the smallest version Put will accept; after
// Invalidate it is one greater than the cleared entry's version so a
// late parse for the cleared version is rejected. file is nil when
// the slot holds a post-Invalidate tombstone.
type parseCacheEntry struct {
	version       int
	file          *File
	minPutVersion int
}

// NewParseCache returns an empty cache ready to be installed on
// engine.Runner.ParseCache.
func NewParseCache() *ParseCache {
	return &ParseCache{entries: make(map[string]parseCacheEntry)}
}

// Get returns the cached *File for path when an entry exists, its
// stored version equals the caller's version, and the slot is not a
// tombstone. Any other state — path absent, present but at a
// different version, or invalidated — is a miss.
//
// A miss leaves the cache unchanged; the caller is expected to
// parse fresh and Put the result.
func (c *ParseCache) Get(path string, version int) (*File, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[path]
	if !ok || e.file == nil || e.version != version {
		return nil, false
	}
	return e.file, true
}

// Put stores f as the cached parse for (path, version). When an
// entry already exists for path, Put writes only when version is at
// least the entry's minPutVersion — a stale Put (older version than
// the current entry, or for a version that Invalidate just cleared)
// is dropped on the floor so two overlapping parses cannot overwrite
// a fresher result with a stale one.
//
// Equal-version Puts on a live entry overwrite (minPutVersion equals
// the entry version, so the comparison admits the same version).
// Two concurrent parses of the same (path, version) both produce
// equivalent *File values; whichever lands last is the one the next
// Get observes. That is a wasted parse, not a correctness bug.
func (c *ParseCache) Put(path string, version int, f *File) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[path]; ok && version < existing.minPutVersion {
		return
	}
	c.entries[path] = parseCacheEntry{
		version:       version,
		file:          f,
		minPutVersion: version,
	}
}

// Invalidate marks the entry for path as cleared and records the
// minimum version Put will accept going forward. A subsequent late
// Put for the cleared version cannot re-insert a stale *File; only a
// Put at a strictly newer version takes the slot. Invalidating a
// path that holds no entry is a no-op (no version watermark to
// remember).
//
// The LSP calls this from didChange, didClose, and
// didChangeWatchedFiles. didChange bumps the document version so the
// next Get misses on the version mismatch alone; the tombstone
// additionally guards against a slow parse from the prior version
// landing after the edit.
func (c *ParseCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	existing, ok := c.entries[path]
	if !ok {
		return
	}
	c.entries[path] = parseCacheEntry{
		version:       existing.version,
		file:          nil,
		minPutVersion: existing.version + 1,
	}
}

// InvalidateAll clears every entry. The LSP calls this when the
// workspace root changes (config reload picks a different
// `.mdsmith.yml`) — every key in the cache was workspace-relative to
// the old root, so the slots no longer match what runLint will
// compute next time. Cheaper than walking every key and equivalent
// in effect.
func (c *ParseCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]parseCacheEntry)
}
