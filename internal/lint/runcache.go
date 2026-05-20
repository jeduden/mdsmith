package lint

import "sync"

// RunCache memoizes per-target-file reads (front matter, include
// adjacency) across every host file processed in one engine.Run pass.
// Cache keys are absolute filesystem paths, so two host files whose
// catalogs match the same target share a single read of that target —
// closing the cross-host redundancy the per-File Memo could not.
//
// The cache is safe for concurrent readers (the parallel file worker
// pool and the LSP's concurrent request goroutines).
//
// A one-shot mdsmith check sees an immutable corpus, so the cache is
// trivially safe there. The LSP keeps one RunCache for the server
// lifetime and calls Invalidate when a document edit could change
// what the next Check would read from disk.
type RunCache struct {
	frontMatter sync.Map // string (absPath) -> *runCacheEntry
	includes    sync.Map // string (absPath) -> *runCacheEntry
}

// runCacheEntry guards a single cache slot so build runs exactly once
// per key even when multiple goroutines race for it.
type runCacheEntry struct {
	once sync.Once
	val  any
}

// NewRunCache returns an empty cache ready to be installed on
// engine.Runner.RunCache.
func NewRunCache() *RunCache {
	return &RunCache{}
}

// FrontMatter returns build's result for absPath, computed at most once
// per absPath in this cache's lifetime. Concurrent callers with the
// same key block on the same once and observe the same value.
func (c *RunCache) FrontMatter(absPath string, build func() any) any {
	return load(&c.frontMatter, absPath, build)
}

// Includes returns build's result for absPath. The value is the list
// of absolute filesystem paths every <?include?> in the file at
// absPath resolves to. Position-independent so two host files whose
// f.FS roots differ can still share the cached adjacency.
func (c *RunCache) Includes(absPath string, build func() []string) []string {
	v := load(&c.includes, absPath, func() any { return build() })
	// v always carries dynamic type []string (the wrapper closure
	// converts build's typed nil to a typed-nil any), so v == nil
	// cannot fire — the assertion succeeds for nil and non-nil
	// slices alike.
	return v.([]string)
}

// Invalidate drops the front-matter and include entries for absPath.
// The LSP calls this from didChange / didSave / didChangeWatchedFiles
// so the next Check that crosses absPath re-reads from disk.
func (c *RunCache) Invalidate(absPath string) {
	c.frontMatter.Delete(absPath)
	c.includes.Delete(absPath)
}

// load is the shared LoadOrStore + sync.Once primitive for both maps.
func load(m *sync.Map, key string, build func() any) any {
	ei, _ := m.LoadOrStore(key, &runCacheEntry{})
	e := ei.(*runCacheEntry)
	e.once.Do(func() { e.val = build() })
	return e.val
}
