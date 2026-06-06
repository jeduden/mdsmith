// Package mdsmith is the public Go engine API for the mdsmith Markdown
// linter. A [Session] owns a workspace, compiled configuration, and
// per-session parse caches, and exposes the core operations — [Session.Check],
// [Session.Fix], and [Session.Kinds] — as thin shims over the engine.
// The same surface mirrors one-to-one into JavaScript via WebAssembly
// (see cmd/mdsmith-wasm), so an in-process JS host consumes one
// contract whether it runs native or in a browser.
//
// The design rationale — the open method namespace, the cache
// contract, and the WASM limits and size budgets — lives in
// docs/background/concepts/engine-api.md. As a public package this is a
// cross-system contract.
package mdsmith

import (
	"bytes"
	"hash/fnv"
	"path/filepath"
	"sync"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	// Register every production rule so rule.All() returns the full
	// set, exactly as cmd/mdsmith/main.go does. The recipesafety rule
	// (MDS040) self-registers only on native (//go:build !wasm), so a
	// WASM build of this package omits it — see the package doc and
	// docs/background/concepts/engine-api.md.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// ConfigSource supplies the configuration for a [Session]. Construct
// one with [ConfigYAML] (inline YAML, the WASM path) or [ConfigPath]
// (a path on the host filesystem, the CLI path). The zero interface is
// not valid; pass ConfigYAML("") for defaults.
type ConfigSource interface {
	loadConfig() (*config.Config, error)
	// configPath returns the on-disk config path when the source is a
	// path, or "" for inline YAML. It gates config-target rules (e.g.
	// MDS040) and is the engine's RootDir anchor.
	configPath() string
}

// alreadyMerged marks a [ConfigSource] whose loadConfig returns a config
// that is already layered over the defaults (and may carry caller
// injected settings). NewSession uses the as-is config for such a
// source instead of re-merging. [ConfigCompiled] is the only
// implementation today.
type alreadyMerged interface {
	alreadyMerged()
}

// ConfigYAML is a [ConfigSource] backed by an inline YAML string. An
// empty string yields a mostly-default config.
type ConfigYAML string

func (c ConfigYAML) loadConfig() (*config.Config, error) {
	return config.ParseBytes([]byte(c))
}

func (c ConfigYAML) configPath() string { return "" }

// ConfigPath is a [ConfigSource] backed by a `.mdsmith.yml` path on the
// host filesystem.
type ConfigPath string

func (c ConfigPath) loadConfig() (*config.Config, error) {
	return config.Load(string(c))
}

func (c ConfigPath) configPath() string { return string(c) }

// ConfigCompiled is a [ConfigSource] backed by an already-merged
// *config.Config plus the path it was loaded from (empty for none). Use
// it when the caller has already loaded and merged the configuration and
// applied its own side effects to it — the CLI's loadConfig injects
// build recipes (for MDS040) and installs the include-extract projector,
// then hands the resulting config straight to a Session. NewSession
// takes a compiled source as-is and does not re-merge it over defaults,
// so those injected settings survive.
func ConfigCompiled(cfg *config.Config, path string) ConfigSource {
	return compiledConfig{cfg: cfg, path: path}
}

// compiledConfig is the [ConfigSource] returned by [ConfigCompiled]. It
// implements alreadyMerged so NewSession skips the defaults merge.
type compiledConfig struct {
	cfg  *config.Config
	path string
}

func (c compiledConfig) loadConfig() (*config.Config, error) { return c.cfg, nil }
func (c compiledConfig) configPath() string                  { return c.path }
func (c compiledConfig) alreadyMerged()                      {}

// SessionOptions configures a [Session].
type SessionOptions struct {
	// Workspace is the filesystem the engine reads through. Required.
	Workspace Workspace
	// Config supplies the rule configuration. Required.
	Config ConfigSource
}

// Session owns a workspace, compiled configuration, and per-session
// caches. Reuse one session across many operations on the same
// workspace; a config change requires Dispose plus a new session.
//
// A Session is safe for concurrent use.
type Session struct {
	ws       Workspace
	cfg      *config.Config
	cfgPath  string
	rules    []rule.Rule
	rootDir  string
	maxBytes int64

	mu sync.Mutex
	// checkCache memoizes Check results by uri then content hash so a
	// repeated Check on identical source skips re-parsing and
	// re-linting (the AST cache the engine API contract promises).
	// Invalidate(uri) drops the uri's entry.
	checkCache map[string]cachedCheck
	// parses counts cache-miss Check passes (each of which parses the
	// source). The test seam parseCount reads it; the bench asserts
	// steady-state stays under half the cold-start time.
	parses int

	// runCache is the engine-owned cross-file read cache shared across
	// every operation on this session, so two host buffers that catalog
	// over the same tree do not each re-read the matched targets. The
	// LSP relies on this surviving across per-keystroke CheckVersion
	// calls; Invalidate drops the changed path's entry. Created once in
	// NewSession.
	runCache *lint.RunCache
	// parseCache memoizes the parsed *lint.File for a document keyed by
	// (uri, version). CheckVersion installs it on the runner so a repeat
	// Check at the same version skips the parse — the plan-216 contract
	// the LSP latency gate depends on. Invalidate drops the path's entry.
	parseCache *lint.ParseCache
	// parseHits counts version-keyed parse-cache hits CheckVersion
	// observed. Test seam (parseCacheHits); not part of the public API.
	parseHits int
}

// cachedCheck is one memoized Check result: the content hash it was
// computed for and the diagnostics it produced.
type cachedCheck struct {
	hash  uint64
	diags []Diagnostic
}

// NewSession compiles the configuration once and returns a ready
// Session. It returns an error when the config source fails to load or
// parse.
func NewSession(opts SessionOptions) (*Session, error) {
	if opts.Workspace == nil {
		opts.Workspace = OSWorkspace{}
	}
	src := opts.Config
	if src == nil {
		src = ConfigYAML("")
	}
	loaded, err := src.loadConfig()
	if err != nil {
		return nil, err
	}
	// Layer the loaded config over built-in defaults exactly as the CLI
	// (cmd/mdsmith loadConfigRaw) and the LSP server do. Without this,
	// a config that omits a rule leaves it disabled rather than at its
	// default-enabled state, so a bare session would lint nothing.
	//
	// A source that has already merged (ConfigCompiled, used by the CLI
	// which merges and then injects build recipes / the include-extract
	// projector onto the result) is taken as-is: re-merging would
	// recompute rule entries and drop those injected settings.
	cfg := loaded
	if _, merged := src.(alreadyMerged); !merged {
		cfg = config.Merge(config.Defaults(), loaded)
	}
	return &Session{
		ws:         opts.Workspace,
		cfg:        cfg,
		cfgPath:    src.configPath(),
		rules:      rule.All(),
		rootDir:    rootDirOf(opts.Workspace),
		maxBytes:   resolveSessionMaxBytes(cfg),
		checkCache: make(map[string]cachedCheck),
		runCache:   lint.NewRunCache(),
		parseCache: lint.NewParseCache(),
	}, nil
}

// resolveSessionMaxBytes resolves the session's input byte cap from the
// config's max-input-size, mirroring cmd/mdsmith's resolveMaxInputBytes
// and the LSP's: an empty setting yields the default 2 MB cap, "0" means
// unlimited, and any other value is the parsed byte count. An
// unparseable value falls back to the default (the CLI surfaces the
// parse error separately at flag time; the session, like the LSP, stays
// lenient and uses the default so a bad config does not wedge linting).
func resolveSessionMaxBytes(cfg *config.Config) int64 {
	if cfg == nil || cfg.MaxInputSize == "" {
		return bytelimit.DefaultMaxInputBytes
	}
	n, err := config.ParseSize(cfg.MaxInputSize)
	if err != nil {
		return bytelimit.DefaultMaxInputBytes
	}
	return n
}

// rootDirOf returns the workspace root to anchor RootDir-dependent
// resolution. An OSWorkspace contributes its Root and an
// OverlayWorkspace (the LSP's workspace) its root — the same directory
// diskPath/Glob resolve against — so the LSP session keys its cross-file
// RunCache by absolute paths, matching the CLI's OSWorkspace rather than
// silently flipping to relative keys. A MemWorkspace has no on-disk root
// (the empty string is correct, leaving cross-file rules to resolve
// root-relative globs against the workspace FS).
func rootDirOf(ws Workspace) string {
	switch w := ws.(type) {
	case OSWorkspace:
		return w.Root
	case *OverlayWorkspace:
		return w.root
	}
	return ""
}

// newRunner builds the engine.Runner that backs Check (and therefore
// Fix's post-fix diagnostics, which route through Check). SourceFS is
// snapshotted per call, so a workspace edit applied through Invalidate
// is visible to the next operation.
func (s *Session) newRunner() *engine.Runner {
	return &engine.Runner{
		Config:           s.cfg,
		Rules:            s.rules,
		StripFrontMatter: frontMatterEnabled(s.cfg),
		RootDir:          s.rootDir,
		MaxInputBytes:    s.maxBytes,
		SourceFS:         s.ws.FS(),
		ConfigPath:       s.cfgPath,
		// Shared cross-file read cache: a catalog/include target read by
		// one operation is reused by the next until Invalidate drops it.
		RunCache: s.runCache,
	}
}

// CheckVersion lints source for uri at the editor's textDocument
// version and returns the engine result. It is the LSP-facing per-
// keystroke entry point: the session's version-keyed parse cache serves
// the parsed document when a prior call already parsed (uri, version),
// so the engine skips re-parsing on a re-lint at the same version (the
// plan-216 contract). An edit bumps the version, which misses the cache
// and re-parses. uri is workspace-relative, matching config globs.
//
// Cross-file rules read through the session workspace's FS view, so an
// open-document overlay applied through Invalidate reaches them — the
// LSP's unsaved-buffer bytes feed catalog/include/link rules.
//
// Native-only: it returns the engine's own Result so the LSP keeps its
// diagnostic partitioning (doc findings vs config-target findings) and
// error surfacing. It is consistent with CheckPaths/CheckSource, which
// also return the engine Result. A WASM host uses the JS-mirrored
// Check.
func (s *Session) CheckVersion(uri string, source []byte, version int) *engine.Result {
	// Record a parse-cache hit for the test seam before delegating: the
	// runner re-probes internally, but counting here keeps the seam off
	// the engine's API. The extra map lookup is negligible against a
	// parse and only on the version path.
	if _, ok := s.parseCache.Get(uri, version); ok {
		s.mu.Lock()
		s.parseHits++
		s.mu.Unlock()
	}
	r := s.newRunner()
	r.ParseCache = s.parseCache
	return r.RunSourceWithVersion(uri, source, version)
}

// Check lints source (the in-memory bytes for uri) and returns its
// diagnostics. A repeated Check on the same (uri, source) reuses the
// cached parse. uri is workspace-relative — config ignore, kind, and
// override globs all match against it.
func (s *Session) Check(uri string, source []byte) ([]Diagnostic, error) {
	h := hashBytes(source)

	s.mu.Lock()
	if c, ok := s.checkCache[uri]; ok && c.hash == h {
		// Return a clone, not the cached slice: a caller that mutates or
		// appends to its result must not poison the cache for the next
		// Check on the same (uri, source).
		diags := cloneDiagnostics(c.diags)
		s.mu.Unlock()
		return diags, nil
	}
	s.parses++
	s.mu.Unlock()

	res := s.newRunner().RunSource(uri, source)
	diags := toDiagnostics(res.Diagnostics)

	s.mu.Lock()
	// checkCache is nil after Dispose. Guard the write so a Check that
	// races or follows Dispose degrades to "no caching" instead of a
	// nil-map-write panic (which, holding s.mu here, would also wedge any
	// concurrent Dispose waiting on the lock). Store a clone so the
	// caller's returned slice never aliases the cache's backing array.
	if s.checkCache != nil {
		s.checkCache[uri] = cachedCheck{hash: h, diags: cloneDiagnostics(diags)}
	}
	s.mu.Unlock()

	if err := firstError(res.Errors); err != nil {
		return diags, err
	}
	return diags, nil
}

// Fix applies every fixable rule allowed by the effective config to
// source and returns the rewritten bytes plus the diagnostics that
// remain afterward. The returned Source equals the input when no rule
// produced an edit (Changed is false). Fix does not write to disk; the
// caller persists Source.
func (s *Session) Fix(uri string, source []byte) (FixResult, error) {
	fixed, err := fixpkg.Source(fixpkg.SourceOptions{
		Config:           s.cfg,
		Rules:            s.rules,
		Path:             uri,
		Source:           source,
		RootDir:          s.rootDir,
		StripFrontMatter: frontMatterEnabled(s.cfg),
		MaxInputBytes:    s.maxBytes,
		SourceFS:         s.ws.FS(),
	})
	if err != nil {
		return FixResult{}, err
	}

	// Re-lint the fixed bytes so the result carries the diagnostics that
	// survive the fix (non-fixable rules, unfixable violations). Route
	// through Check, not a fresh full runner, so the session's parse and
	// check caches absorb the work (footgun 4: avoid re-linting twice).
	// When the fix made no edit, fixed == source, so this Check hits the
	// cache if the caller already Checked this source — a no-op Fix on a
	// clean buffer no longer pays for a second lint. The cache is keyed
	// by content hash, so the fixed bytes (changed or not) reuse or
	// populate the correct entry.
	diags, err := s.Check(uri, fixed)
	if err != nil {
		return FixResult{
			Source:      string(fixed),
			Changed:     !bytes.Equal(fixed, source),
			Diagnostics: diags,
		}, err
	}

	return FixResult{
		Source:      string(fixed),
		Changed:     !bytes.Equal(fixed, source),
		Diagnostics: diags,
	}, nil
}

// FixRule applies only the named fixable rules to source and returns
// the rewritten bytes plus a Changed flag. It is the LSP per-rule
// quick-fix entry point (today's fix.SourceWithRules): a lightbulb that
// fixes one rule's violations rewrites the document through this without
// disturbing other rules. An empty names slice is a no-op.
//
// FixRule does not re-lint — the quick-fix path needs only the bytes and
// whether they changed — so Diagnostics on the result is always nil.
// Cross-file rules read through the session workspace's FS view, so an
// open-document overlay (Invalidate) reaches them.
func (s *Session) FixRule(uri string, source []byte, names []string) (FixResult, error) {
	fixed, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           s.cfg,
		Rules:            s.rules,
		Path:             uri,
		Source:           source,
		RootDir:          s.rootDir,
		StripFrontMatter: frontMatterEnabled(s.cfg),
		MaxInputBytes:    s.maxBytes,
		SourceFS:         s.ws.FS(),
	}, names)
	if err != nil {
		return FixResult{}, err
	}
	return FixResult{
		Source:  string(fixed),
		Changed: !bytes.Equal(fixed, source),
	}, nil
}

// Kinds resolves the kind list and effective rule configuration for
// uri, including per-leaf provenance. It reads the file's bytes through
// the workspace to parse its front matter (the `kinds:` list and any
// `fields-present:` selector inputs); a missing file resolves against
// path-pattern and override globs alone.
func (s *Session) Kinds(uri string) (KindResolution, error) {
	fmKinds, fmFields, err := s.frontMatterFor(uri)
	if err != nil {
		return KindResolution{}, err
	}
	return toKindResolution(s.ResolveFile(uri, fmKinds, fmFields)), nil
}

// ResolveFile resolves the kind list and per-rule effective config for
// uri against the session's compiled config, given the file's
// already-parsed front-matter kinds and fields (pass nil when there are
// none). It returns the raw *config.FileResolution, including the
// per-rule merge chain the CLI's `kinds why` walks.
//
// Native-only: it returns an internal config type. The CLI reads and
// validates front matter itself (its error UX differs from the
// session's lenient unsaved-buffer handling) and hands the parsed
// inputs here; [Session.Kinds] is the JS-mirrored sibling that reads
// front matter through the workspace and returns the public JSON shape.
func (s *Session) ResolveFile(uri string, fmKinds []string, fmFields map[string]any) *config.FileResolution {
	return config.ResolveFile(s.cfg, uri, fmKinds, fmFields)
}

// frontMatterFor reads uri through the workspace and parses its
// front-matter kinds and fields. A workspace miss is not an error: the
// file may be unsaved or new, so resolution proceeds with no
// front-matter inputs.
func (s *Session) frontMatterFor(uri string) ([]string, map[string]any, error) {
	source, err := s.ws.ReadFile(uri)
	if err != nil {
		// Treat a missing file as "no front matter" rather than an
		// error so Kinds works for unsaved buffers.
		return nil, nil, nil //nolint:nilerr // intentional: missing file means empty front matter
	}
	// Extract the front-matter block directly. lint.StripFrontMatter is
	// the same extraction NewFileFromSource performs; the full document
	// parse it also does is not needed here, and skipping it avoids a
	// never-failing error branch (NewFile does not return errors).
	var fm []byte
	if frontMatterEnabled(s.cfg) {
		fm, _ = lint.StripFrontMatter(source)
	}
	fmKinds, err := lint.ParseFrontMatterKinds(fm)
	if err != nil {
		return nil, nil, err
	}
	var fmFields map[string]any
	if config.NeedsFieldsForFile(s.cfg, uri) {
		fmFields, err = lint.ParseFrontMatterFields(fm)
		if err != nil {
			return nil, nil, err
		}
	}
	return fmKinds, fmFields, nil
}

// Capabilities returns the method names this build supports. The list
// holds method names, never rule IDs, and is the same in Go and JS for
// one build. Callers feature-detect with a membership test before
// calling a method.
func (s *Session) Capabilities() []string {
	return capabilityList()
}

// capabilityList is the single source of truth for the method names a
// session supports. The WASM bridge reads the same list so Go and JS
// stay in lock-step; a test asserts the JS session method set matches
// it name-for-name.
func capabilityList() []string {
	return []string{"check", "fix", "kinds"}
}

// Invalidate signals that uri changed. With a content argument it
// rewrites uri in the workspace (when the workspace implements the
// mutable Set/Delete overlay — MemWorkspace and the LSP's buffer
// overlay do) so the next cross-file Check reads the new bytes; a bare
// OSWorkspace ignores content and re-reads disk. A no-content call on a
// mutable workspace deletes the file (it was removed).
//
// It then drops every cached Check result, not only uri's: a changed
// file can affect any file that reads it through a cross-file rule
// (catalog, include, links), and the session keeps no dependency graph,
// so serving a cached dependent could return a stale result. The parse
// reuse the cache buys is a within-session optimisation; correctness
// after an edit wins.
func (s *Session) Invalidate(uri string, content ...[]byte) {
	// Route the open-document bytes through the Workspace's mutable
	// interface (Set/Delete) rather than a hardcoded *MemWorkspace type
	// assertion, so any buffer-overlay workspace — the LSP's, which
	// shadows disk with unsaved-buffer content — receives the edit and
	// its cross-file rules read the new bytes (footgun 3). An OSWorkspace
	// (no Set/Delete) re-reads disk, so only the caches below drop.
	if mw, ok := s.ws.(mutableWorkspace); ok {
		switch {
		case len(content) > 0:
			mw.Set(uri, content[0])
		default:
			mw.Delete(uri)
		}
	}
	// Drop the engine caches for the changed path so the next operation
	// re-reads and re-parses it: the cross-file read cache (keyed by the
	// absolute path catalog/include compute) and the version-keyed parse
	// cache (keyed by the workspace-relative uri).
	s.runCache.Invalidate(s.absPath(uri))
	s.parseCache.Invalidate(uri)
	s.mu.Lock()
	clear(s.checkCache)
	s.mu.Unlock()
}

// InvalidateWikilinks drops the cross-file read cache's wikilink index
// so the next Check rebuilds the candidate set. The LSP calls it when a
// watched file is created or deleted: a wikilink like `[[NewPage]]`
// would otherwise resolve against the pre-change set. It is the
// workspace-shape sibling of [Session.Invalidate], which drops a single
// path's cached content.
func (s *Session) InvalidateWikilinks() {
	s.runCache.InvalidateWikilinks()
}

// absPath maps a workspace-relative uri to the absolute path the engine
// read cache keys catalog/include reads by. With no rootDir (a
// MemWorkspace, or an unrooted OSWorkspace) the uri is returned
// unchanged — the cache keys it the same way the rules do.
func (s *Session) absPath(uri string) string {
	if s.rootDir == "" || filepath.IsAbs(uri) {
		return uri
	}
	return filepath.Join(s.rootDir, filepath.FromSlash(uri))
}

// Dispose releases the session's caches. The session must not be used
// afterward. It is safe to call more than once.
func (s *Session) Dispose() {
	s.mu.Lock()
	s.checkCache = nil
	s.mu.Unlock()
}

// parseCount returns the number of cache-miss Check passes (each of
// which parsed the source). It is a test/bench seam, not part of the
// public API.
func (s *Session) parseCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parses
}

// parseCacheHits returns the number of version-keyed parse-cache hits
// CheckVersion observed. Test seam, not part of the public API.
func (s *Session) parseCacheHits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parseHits
}

// frontMatterEnabled reports whether front-matter stripping is on for
// the compiled config. Mirrors cmd/mdsmith's helper of the same name.
// A nil cfg is treated as the default (enabled).
func frontMatterEnabled(cfg *config.Config) bool {
	return cfg == nil || cfg.FrontMatter == nil || *cfg.FrontMatter
}

// hashBytes returns a fast non-cryptographic content hash for cache
// keying. FNV-1a is enough: a collision only risks a stale Check result
// for one uri, and Invalidate is the documented escape hatch.
func hashBytes(b []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

func firstError(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return errs[0]
}

// cloneDiagnostics returns a shallow copy of the slice so a caller
// cannot append into or overwrite the cache's backing array. The
// Diagnostic elements are copied by value; their inner slices
// (SourceLines, RelatedLocations) and pointer (Explanation) are
// shared, which is safe because callers treat results as read-only and
// the engine never mutates a diagnostic after producing it. nil maps to nil so the
// public no-diagnostics contract (nil, not []) is preserved.
func cloneDiagnostics(in []Diagnostic) []Diagnostic {
	if in == nil {
		return nil
	}
	out := make([]Diagnostic, len(in))
	copy(out, in)
	return out
}
