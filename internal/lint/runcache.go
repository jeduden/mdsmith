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
	frontMatter  sync.Map // string (absPath) -> *runCacheEntry
	includes     sync.Map // string (absPath) -> *runCacheEntry
	anchors      sync.Map // string (absPath) -> *anchorEntry
	wikilinks    sync.Map // string (root key) -> *runCacheEntry
	parsedSchema sync.Map // string (absPath) -> *runCacheEntry
	compiledCUE  sync.Map // string (CUE source) -> *runCacheEntry
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

// Anchors returns the cached anchor set for absPath, computing it
// at most once via build. The build callback returns an error
// alongside the anchor set; on error, nothing is cached and the
// next call retries (matches the per-Check anchorCache map's
// previous behaviour, where a read failure produced a diagnostic
// on the host file but did not poison the cache for siblings).
//
// Callers reach this slot via the cross-file-reference rule's
// per-target lookup; on link-heavy corpora it collapses the
// per-host-file goldmark parse + AST walk to one walk per (Run,
// target).
func (c *RunCache) Anchors(absPath string, build func() (map[string]bool, error)) (map[string]bool, error) {
	ei, _ := c.anchors.LoadOrStore(absPath, &anchorEntry{})
	e := ei.(*anchorEntry)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.done {
		return e.anchors, nil
	}
	anchors, err := build()
	if err != nil {
		// Do not flip the done flag on a read error — the next
		// caller retries. A successful build is recorded once.
		return nil, err
	}
	e.anchors = anchors
	e.done = true
	return anchors, nil
}

// anchorEntry guards a single absPath's anchor slot. The mutex
// serialises concurrent peekers; once.Do was not enough because
// the build path returns an error and a successful build must
// be cacheable while a failed one stays retryable.
type anchorEntry struct {
	mu      sync.Mutex
	done    bool
	anchors map[string]bool
}

// Wikilinks returns build's result keyed by rootKey, computed at
// most once per rootKey in this cache's lifetime. Concurrent
// callers with the same key block on the same once and observe
// the same value.
//
// The value carries dynamic type any — the canonical caller is
// linkgraph.WikilinkIndexFor, which stores a
// *linkgraph.WikilinkIndex so a workspace walked for one host
// file serves every later file in the run. MDS027 routes
// through that helper via the engine's RunCache; `mdsmith list
// backlinks` calls the helper directly without a cache because
// it is one-shot. Either way the build/cache contract sits in
// one place.
func (c *RunCache) Wikilinks(rootKey string, build func() any) any {
	return load(&c.wikilinks, rootKey, build)
}

// ParsedSchema returns build's result for absPath, computed at most
// once per absPath in this cache's lifetime. The value carries
// dynamic type any so the requiredstructure rule (MDS020) can store
// its package-private *parsedSchema (or a (parsedSchema, error) tuple)
// without leaking the type into the lint package. Concurrent callers
// with the same key block on the same once and observe the same
// value.
//
// Build errors are cached intentionally: a malformed schema must
// not be re-parsed on every host file that references it. Callers
// store an error in the returned `any` so the next lookup is a
// map hit. The LSP must call Invalidate(schemaPath) when the user
// edits the schema so the next Check re-parses it.
//
// MDS020 reaches this slot via f.RunCache; on a corpus where N
// host files all reference one schema, the per-Check parseSchema
// chain (schema markdown parse, AST walk, frontmatter CUE-derive)
// collapses from N runs to 1 — closing the parity-gap profile
// that plan 195 documents as the biggest default-rule hot spot.
func (c *RunCache) ParsedSchema(absPath string, build func() any) any {
	return load(&c.parsedSchema, absPath, build)
}

// CompiledCUE returns build's result for source — the CUE source
// string itself, not a file path. The same expression can come
// from a schema file or from an inline `schema:` block; using the
// source as the key collapses both into one compile per Run.
//
// The cached value carries dynamic type any so the caller can
// store a wrapper that pairs the produced cue.Value with the
// cue.Context that produced it (cue values are tied to their
// context and must not cross contexts).
//
// MDS020's validateCUESchemaSyntax / validateFrontMatterCUE call
// `cuecontext.New().CompileString(schema)` on every Check; with
// this slot the compile runs once per unique CUE source per Run,
// regardless of how many host files share the schema.
func (c *RunCache) CompiledCUE(source string, build func() any) any {
	return load(&c.compiledCUE, source, build)
}

// Invalidate drops the front-matter, include, anchor, and
// parsed-schema entries for absPath. The LSP calls this from
// didChange / didSave / didChangeWatchedFiles so the next Check
// that crosses absPath re-reads from disk.
//
// Wikilink indices are NOT invalidated per absPath because a file
// rename or creation could change the resolution of any wikilink in
// the workspace; the LSP must InvalidateWikilinks (or build a
// fresh RunCache) when the filesystem layout changes.
//
// The CompiledCUE slot is NOT invalidated either: its key is the
// CUE source string, not a file path, so editing a schema file
// changes its parsed-schema entry but the resulting CUE source —
// if unchanged — still maps to the same valid cached value. The
// LSP installs a fresh RunCache on workspace reload, which clears
// it then.
func (c *RunCache) Invalidate(absPath string) {
	c.frontMatter.Delete(absPath)
	c.includes.Delete(absPath)
	c.anchors.Delete(absPath)
	c.parsedSchema.Delete(absPath)
}

// InvalidateWikilinks clears every cached wikilink index. The LSP
// calls this when the workspace tree changes (file create/delete/
// rename) so the next resolution walks afresh.
func (c *RunCache) InvalidateWikilinks() {
	c.wikilinks.Range(func(k, _ any) bool {
		c.wikilinks.Delete(k)
		return true
	})
}

// load is the shared LoadOrStore + sync.Once primitive for both maps.
func load(m *sync.Map, key string, build func() any) any {
	ei, _ := m.LoadOrStore(key, &runCacheEntry{})
	e := ei.(*runCacheEntry)
	e.once.Do(func() { e.val = build() })
	return e.val
}
