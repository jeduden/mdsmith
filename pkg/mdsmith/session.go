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
	"hash/fnv"
	"sync"

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
	cfg := config.Merge(config.Defaults(), loaded)
	return &Session{
		ws:         opts.Workspace,
		cfg:        cfg,
		cfgPath:    src.configPath(),
		rules:      rule.All(),
		rootDir:    rootDirOf(opts.Workspace),
		maxBytes:   lint.DefaultMaxInputBytes,
		checkCache: make(map[string]cachedCheck),
	}, nil
}

// rootDirOf returns the workspace root to anchor RootDir-dependent
// resolution. An OSWorkspace contributes its Root; a MemWorkspace has
// no on-disk root (the empty string is correct, leaving cross-file
// rules to resolve root-relative globs against the workspace FS).
func rootDirOf(ws Workspace) string {
	if osw, ok := ws.(OSWorkspace); ok {
		return osw.Root
	}
	return ""
}

// newRunner builds the engine.Runner shared by Check and by Fix's
// post-fix re-lint, so both paths lint with identical configuration.
// SourceFS is snapshotted per call, so a workspace edit applied through
// Invalidate is visible to the next operation.
func (s *Session) newRunner() *engine.Runner {
	return &engine.Runner{
		Config:           s.cfg,
		Rules:            s.rules,
		StripFrontMatter: frontMatterEnabled(s.cfg),
		RootDir:          s.rootDir,
		MaxInputBytes:    s.maxBytes,
		SourceFS:         s.ws.FS(),
		ConfigPath:       s.cfgPath,
	}
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

	// Re-lint the fixed bytes so the result carries the diagnostics
	// that survive the fix (non-fixable rules, unfixable violations).
	// This does not poison the Check cache: the fixed bytes hash
	// differently, and a later Check on them reuses the entry.
	res := s.newRunner().RunSource(uri, fixed)

	return FixResult{
		Source:      string(fixed),
		Changed:     string(fixed) != string(source),
		Diagnostics: toDiagnostics(res.Diagnostics),
	}, firstError(res.Errors)
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
	res := config.ResolveFile(s.cfg, uri, fmKinds, fmFields)
	return toKindResolution(res), nil
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
// rewrites uri in the workspace (when the workspace is a MemWorkspace)
// so the next cross-file Check reads the new bytes; an OSWorkspace
// ignores content and re-reads disk. A no-content call on a
// MemWorkspace deletes the file (it was removed).
//
// It then drops every cached Check result, not only uri's: a changed
// file can affect any file that reads it through a cross-file rule
// (catalog, include, links), and the session keeps no dependency graph,
// so serving a cached dependent could return a stale result. The parse
// reuse the cache buys is a within-session optimisation; correctness
// after an edit wins.
func (s *Session) Invalidate(uri string, content ...[]byte) {
	if mw, ok := s.ws.(*MemWorkspace); ok {
		switch {
		case len(content) > 0:
			mw.Set(uri, content[0])
		default:
			mw.Delete(uri)
		}
	}
	s.mu.Lock()
	clear(s.checkCache)
	s.mu.Unlock()
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

// frontMatterEnabled reports whether front-matter stripping is on for
// the compiled config. Mirrors cmd/mdsmith's helper of the same name.
func frontMatterEnabled(cfg *config.Config) bool {
	return cfg.FrontMatter == nil || *cfg.FrontMatter
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
