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

	// uniqueFieldIndex memoizes MDS069's per-scope value→first-file
	// index. Keys encode a rule scope (field + globs), not a path.
	// uniqueFieldScopes mirrors each entry's ScopeInvalidator so
	// Invalidate can keep indexes whose scope the edited path cannot
	// touch; it is written only AFTER the entry's once completes
	// (the same post-once discipline as schemaIncludes) so reads
	// never race an in-flight build. Entries without a registered
	// scope drop on every invalidation — the safe default.
	uniqueFieldIndex  sync.Map // string (scope key) -> *runCacheEntry
	uniqueFieldScopes sync.Map // string (scope key) -> ScopeInvalidator

	// schemaDependents maps a fragment path to the set of schema
	// paths whose ParsedSchema slot reached it via <?include?>.
	// Invalidate(fragment) walks this set so dependent schemas are
	// evicted alongside the fragment. The inner *sync.Map acts as a
	// concurrent string-set: keys are dependent schemaPaths, values
	// are an empty struct sentinel. Built lazily as ParsedSchema
	// slots populate; the LSP's didChange path on a fragment then
	// transitively invalidates every schema that reached it.
	schemaDependents sync.Map // string (fragmentPath) -> *sync.Map

	// schemaIncludes / schemaCUESources mirror ParsedSchemaMetadata
	// into dedicated sync.Maps so Invalidate reads metadata without
	// peeking at runCacheEntry.val — that would race with the slot's
	// sync.Once during a first-time build. The slots are written
	// AFTER ParsedSchema's load returns (so after the once completes)
	// and read by Invalidate via sync.Map.Load, which gives the
	// happens-before guarantee. A missing entry just means no
	// metadata is registered yet; eviction degrades to a no-op rather
	// than races on a partial value.
	schemaIncludes   sync.Map // string (absPath) -> []string
	schemaCUESources sync.Map // string (absPath) -> []string
}

// runCacheEntry guards a single cache slot so build runs exactly once
// per key even when multiple goroutines race for it.
type runCacheEntry struct {
	once sync.Once
	val  any
}

// ParsedSchemaMetadata is the optional interface a parsed-schema
// cache value (whatever its concrete type) implements so RunCache.
// Invalidate can drop downstream entries that depend on the
// invalidated schema. The rule package's schemaParseResult satisfies
// it; the lint package only sees the surface.
//
//   - SchemaIncludes returns absolute paths of every fragment the
//     schema's <?include?> directives reached. Invalidate(fragment)
//     uses the reverse of this set (built lazily as ParsedSchema slots
//     populate) to evict every schema that includes fragment.
//   - SchemaCUESources returns every distinct CUE source string the
//     schema's frontmatter produced. Invalidate(schemaPath) drops the
//     matching CompiledCUE entries so an LSP edit that retypes a
//     frontmatter constraint does not leak compiled values forever.
//
// Both methods may return nil for a parsed schema that pulled in no
// fragments or has no frontmatter CUE source.
type ParsedSchemaMetadata interface {
	SchemaIncludes() []string
	SchemaCUESources() []string
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

// ScopeInvalidator scopes the unique-field-index slot's response to
// Invalidate. A cached value that implements it is dropped only when
// the invalidated path can fall inside its scope; values without it
// drop on every invalidation — the safe default.
type ScopeInvalidator interface {
	MatchesInvalidatedPath(absPath string) bool
}

// UniqueFieldIndex returns build's result for key, computed at most
// once per key in this cache's lifetime. Keys encode a rule's whole
// uniqueness scope (field plus include/exclude globs). When the
// built value implements ScopeInvalidator it is registered for
// targeted invalidation; the registration happens after load
// returns (post-once), so Invalidate's reads never race the build.
func (c *RunCache) UniqueFieldIndex(key string, build func() any) any {
	v := load(&c.uniqueFieldIndex, key, build)
	// Register (or refresh) the scope when missing or when the
	// entry was rebuilt under the same key — a racing Invalidate
	// between load and Store could otherwise leave the scope map
	// pointing at a previous index instance. Scopes for one key
	// are equivalent (the key encodes the settings), so a stale
	// pointer answers correctly; the refresh keeps the invariant
	// "the registered scope belongs to the live entry" honest.
	if si, ok := v.(ScopeInvalidator); ok {
		if cur, found := c.uniqueFieldScopes.Load(key); !found || cur != any(si) {
			c.uniqueFieldScopes.Store(key, si)
		}
	}
	return v
}

// dropUniqueFieldIndexes clears unique-field index entries that the
// edited path could affect. An entry with a registered
// ScopeInvalidator survives when absPath falls outside its scope —
// an edit to an unrelated file must not force an index rebuild on
// the next lint pass. Entries without a scope, or any call with an
// empty absPath, drop unconditionally.
func (c *RunCache) dropUniqueFieldIndexes(absPath string) {
	c.uniqueFieldIndex.Range(func(k, _ any) bool {
		if siv, ok := c.uniqueFieldScopes.Load(k); ok && absPath != "" {
			if !siv.(ScopeInvalidator).MatchesInvalidatedPath(absPath) {
				return true
			}
		}
		c.uniqueFieldIndex.Delete(k)
		c.uniqueFieldScopes.Delete(k)
		return true
	})
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
func (c *RunCache) Anchors(absPath string, build func() (map[string]struct{}, error)) (map[string]struct{}, error) {
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
	anchors map[string]struct{}
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
// When build's return value satisfies ParsedSchemaMetadata, the
// cache also registers absPath as a dependent of every path in
// SchemaIncludes() on the reverse-include index. Invalidate(fragment)
// then evicts every schema that reached fragment, closing the
// stale-fragment gap the LSP would otherwise observe.
//
// MDS020 reaches this slot via f.RunCache; on a corpus where N
// host files all reference one schema, the per-Check parseSchema
// chain (schema markdown parse, AST walk, frontmatter CUE-derive)
// collapses from N runs to 1 — closing the parity-gap profile
// that plan 195 documents as the biggest default-rule hot spot.
func (c *RunCache) ParsedSchema(absPath string, build func() any) any {
	v := load(&c.parsedSchema, absPath, build)
	if meta, ok := v.(ParsedSchemaMetadata); ok {
		c.registerSchemaMetadata(absPath, meta)
	}
	return v
}

// registerSchemaMetadata captures the parsed schema's includes and
// CUE sources into dedicated sync.Maps (so Invalidate can read them
// race-free) and adds absPath as a dependent of every include
// fragment on the reverse-include index. Idempotent under concurrent
// ParsedSchema calls for the same absPath: the inner sync.Map's
// LoadOrStore re-uses the existing set and a re-Store of the same
// metadata slice is a benign overwrite.
func (c *RunCache) registerSchemaMetadata(absPath string, meta ParsedSchemaMetadata) {
	includes := meta.SchemaIncludes()
	c.schemaIncludes.Store(absPath, includes)
	c.schemaCUESources.Store(absPath, meta.SchemaCUESources())
	c.registerSchemaIncludes(absPath, includes)
}

// registerSchemaIncludes adds schemaPath as a dependent of every
// fragment in includes on the reverse-include index. Idempotent
// under concurrent ParsedSchema calls for the same schemaPath: the
// inner sync.Map's LoadOrStore re-uses the existing set, and
// re-adding the same dependent key is a no-op.
//
// The verify-and-retry loop closes a race with Invalidate's
// empty-set cleanup: Invalidate's CompareAndDelete on the outer
// schemaDependents map only compares the outer-value pointer,
// which stays the same even when a concurrent register adds to
// the inner *sync.Map. Without the retry, this sequence loses
// the new dependent:
//
//  1. T1 register: LoadOrStore("frag") → setI (existing)
//  2. T2 invalidate: empty-check on setI → empty → CompareAndDelete
//     drops the outer entry
//  3. T1 register: set.Store(schemaPath) → lands on the orphaned
//     setI, never reachable from schemaDependents again
//
// The retry detects step 3's orphaning by re-Loading the outer
// entry after Store and confirming it still points at setI.
// On mismatch, the loop re-issues LoadOrStore, which creates a
// fresh set and re-registers. The retry cap (8) is well above
// any plausible race depth — register-vs-invalidate is bounded
// by LSP edit rate.
func (c *RunCache) registerSchemaIncludes(schemaPath string, includes []string) {
	for _, frag := range includes {
		if frag == "" {
			continue
		}
		for retry := 0; retry < 8; retry++ {
			setI, _ := c.schemaDependents.LoadOrStore(frag, &sync.Map{})
			set := setI.(*sync.Map)
			set.Store(schemaPath, struct{}{})
			if current, ok := c.schemaDependents.Load(frag); ok && current == setI {
				break
			}
		}
	}
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
// Schema dependency invalidation: when absPath's ParsedSchema slot
// carries ParsedSchemaMetadata,
//
//   - every CUE source string in SchemaCUESources() is dropped from
//     the compiledCUE slot (conservatively — a sibling schema with
//     the same CUE source will recompile on its next lookup, which
//     is cheap), and
//   - every schema in schemaDependents[absPath] is recursively
//     Invalidated (their cached parses depended on absPath as a
//     fragment, so a fragment edit must propagate). The reverse-
//     include graph CAN contain cycles — extractSchemaHeadings
//     surfaces both edges of an include cycle on its error path so
//     a legal LSP mid-edit state (schemaA <?include B?> with
//     schemaB <?include A?>) registers both edges. Recursion is
//     made finite by the per-call visited set the public Invalidate
//     entry point seeds and the recursive invalidate worker
//     consults; each path is invalidated at most once per top-level
//     call.
//
// The reverse-index edges that name absPath as a dependent (i.e.
// the entry on each include path's set) are dropped after the
// recursion so the absPath's metadata can still be read first. The
// recursion-then-delete order is load-bearing: Invalidate reads the
// parsed-schema slot for its dependency metadata before dropping
// it.
//
// Wikilink indices are NOT invalidated per absPath because a file
// rename or creation could change the resolution of any wikilink in
// the workspace; the LSP must InvalidateWikilinks (or build a
// fresh RunCache) when the filesystem layout changes.
func (c *RunCache) Invalidate(absPath string) {
	c.invalidate(absPath, map[string]bool{})
}

// invalidate is the recursive worker for Invalidate. The visited set
// tracks every path the current top-level call has already touched
// so a cyclic reverse-include graph (legal mid-edit LSP state — a
// schema whose <?include?> chain self-references via a partial-parse
// register on the cycle-error path) terminates on the second
// encounter. The set is per-call (allocated by the public
// Invalidate entry point) so independent Invalidate calls do not
// share visited state.
func (c *RunCache) invalidate(absPath string, visited map[string]bool) {
	if visited[absPath] {
		return
	}
	visited[absPath] = true

	c.frontMatter.Delete(absPath)
	c.includes.Delete(absPath)
	c.anchors.Delete(absPath)

	c.dropUniqueFieldIndexes(absPath)

	// Read the schema's includes + cueSources from the dedicated
	// sync.Maps. Both are written AFTER ParsedSchema's load returns
	// (post-sync.Once) and read here via sync.Map.Load, so there is
	// no race with an in-flight build. A miss (slot never populated,
	// or already dropped by a sibling Invalidate via the dependents
	// walk) leaves the slices nil and the rest of the eviction is a
	// no-op.
	var includes, cueSources []string
	if v, ok := c.schemaIncludes.Load(absPath); ok {
		includes, _ = v.([]string)
	}
	if v, ok := c.schemaCUESources.Load(absPath); ok {
		cueSources, _ = v.([]string)
	}

	// Drop every CompiledCUE entry the parsed schema produced. The
	// CUE source is the cache key, so two schemas with identical
	// front matter share one slot — invalidating absPath drops the
	// slot for both, and the sibling recompiles on its next lookup
	// (one CUE compile per surviving schema is the worst case).
	for _, src := range cueSources {
		c.compiledCUE.Delete(src)
	}

	// Walk the reverse-include index: every schema that included
	// absPath as a fragment must be invalidated transitively. Drain
	// the set into a slice first so the recursive invalidate calls
	// (which also touch schemaDependents during their own
	// fragment-edge cleanup) cannot race with this iteration. The
	// reverse-index entry for absPath is dropped after the walk so
	// a re-populated schema's next register re-creates the set with
	// only the live dependents. The visited set carried through the
	// recursion is the cycle guard — a dependent that has already
	// been invalidated in this top-level call is skipped.
	if setI, ok := c.schemaDependents.Load(absPath); ok {
		set := setI.(*sync.Map)
		var deps []string
		set.Range(func(k, _ any) bool {
			deps = append(deps, k.(string))
			return true
		})
		for _, dep := range deps {
			c.invalidate(dep, visited)
		}
		c.schemaDependents.Delete(absPath)
	}

	// Drop the parsed-schema slot and its mirrored metadata.
	c.parsedSchema.Delete(absPath)
	c.schemaIncludes.Delete(absPath)
	c.schemaCUESources.Delete(absPath)

	// Drop the reverse-index edges where absPath appears as a
	// dependent — for each fragment in absPath's includes, remove
	// absPath from that fragment's dependent set. Leaves the
	// fragment's own slot intact (it is a sibling document, not the
	// invalidated one); only the back-pointer to absPath is
	// removed. When that removal empties the set, CompareAndDelete
	// drops the schemaDependents entry too so long-lived LSP
	// sessions do not accumulate empty *sync.Maps. CompareAndDelete
	// is the race-safe primitive: if a concurrent
	// registerSchemaMetadata re-Stored a dependent between our
	// Range and the delete, the value in schemaDependents now
	// differs from setI and the delete is skipped, preserving the
	// new entry.
	for _, frag := range includes {
		if frag == "" {
			continue
		}
		setI, ok := c.schemaDependents.Load(frag)
		if !ok {
			continue
		}
		set := setI.(*sync.Map)
		set.Delete(absPath)
		empty := true
		set.Range(func(_, _ any) bool {
			empty = false
			return false
		})
		if empty {
			c.schemaDependents.CompareAndDelete(frag, setI)
		}
	}
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
