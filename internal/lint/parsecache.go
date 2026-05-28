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
type parseCacheEntry struct {
	version int
	file    *File
}

// NewParseCache returns an empty cache ready to be installed on
// engine.Runner.ParseCache.
func NewParseCache() *ParseCache {
	return &ParseCache{entries: make(map[string]parseCacheEntry)}
}

// Get returns the cached *File for path when an entry exists and
// its stored version equals the caller's version. Any other state —
// path absent, or present but at a different version — is a miss.
//
// A miss leaves the cache unchanged; the caller is expected to
// parse fresh and Put the result.
func (c *ParseCache) Get(path string, version int) (*File, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[path]
	if !ok || e.version != version {
		return nil, false
	}
	return e.file, true
}

// Put stores f as the cached parse for (path, version). When an
// entry already exists for path, Put writes only when version is
// greater than or equal to the existing version — a stale Put
// (older version than the current entry) is dropped on the floor so
// two overlapping parses cannot overwrite a fresher result with a
// stale one.
//
// Equal-version Puts overwrite. Two concurrent parses of the same
// (path, version) both produce equivalent *File values; whichever
// lands last is the one the next Get observes. That is a wasted
// parse, not a correctness bug.
func (c *ParseCache) Put(path string, version int, f *File) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[path]; ok && version < existing.version {
		return
	}
	c.entries[path] = parseCacheEntry{version: version, file: f}
}

// Invalidate drops the entry for path. The LSP calls this from
// didClose (buffer gone) and didChangeWatchedFiles (on-disk content
// changed under us); didChange bumps the version so the next Get
// misses without an explicit Invalidate, but invalidating there
// drops the dead older entry promptly rather than letting it sit.
//
// Invalidating a path that holds no entry is a no-op.
func (c *ParseCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, path)
}
