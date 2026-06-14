package engine

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/explain"
	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

// sourceBufPool recycles the per-file source-read buffer across the
// engine's lintFile passes. Every file's source bytes used to be a
// fresh allocation that died with the parsed File — the top allocator
// on a `mdsmith check` run (~10% of alloc_space on the repo corpus).
// lintFile draws one *[]byte here, reads the file into it via
// bytelimit.ReadFileLimitedInto, and returns it from the same release()
// closure that recycles the parse arena: the File, its Lines, and any
// output aliasing Source are all dead by then. Buffers are resliced to
// zero length before going back so a single large file does not pin
// megabytes; their (possibly grown) capacity is reused on the next draw.
//
// Only lintFile uses this. RunSource (LSP ParseCache), RunCache target
// loads, and mdsmith export keep the plain allocating read because
// their Files outlive the call.
var sourceBufPool = sync.Pool{New: func() any { b := make([]byte, 0, 8192); return &b }}

// Runner drives the linting pipeline: for each file it reads the content,
// builds a File (parsing the AST once), determines the effective rule
// configuration, runs enabled rules, and collects diagnostics.
type Runner struct {
	Config           *config.Config
	Rules            []rule.Rule
	StripFrontMatter bool
	Logger           *vlog.Logger
	// RootDir is the project root directory (parent of .mdsmith.yml).
	// Used by rules that need to read files relative to the project root.
	RootDir string
	// MaxInputBytes is the maximum file size in bytes before a file is
	// skipped with an error. Zero or negative means unlimited.
	MaxInputBytes int64
	// Explain, when true, attaches per-leaf rule provenance to each
	// diagnostic so output formatters can render an explanation trailer.
	Explain bool
	// SkipSourceContext, when true, suppresses per-diagnostic
	// SourceLines population. Set it for callers that never render the
	// source window (the check benchmark/gate, machine output that
	// omits it) to avoid its large per-diagnostic allocation. Default
	// false preserves the CLI text formatter's context display.
	SkipSourceContext bool
	// ConfigPath is the path to the loaded .mdsmith.yml. When set,
	// config-target rules (rule.ConfigTarget) are run once against a
	// synthetic lint.File for this path before per-file processing.
	ConfigPath string
	// SourceFS, when non-nil, is set as lint.File.FS for RunSource so
	// in-memory linting (e.g. an LSP buffer) sees the same filesystem
	// view processFile constructs for on-disk runs. Rules like
	// include/catalog short-circuit on a nil FS; without this hook
	// LSP diagnostics drift from the CLI's. Run() (path-based)
	// ignores this field and continues to derive FS from filepath.Dir
	// per file.
	SourceFS fs.FS
	// Concurrency controls how many files Run lints in parallel.
	// Zero or negative means "use runtime.GOMAXPROCS"; 1 forces the
	// sequential path; n>1 uses n workers. The worker count is
	// clamped to the file count. Output is merged in input order and
	// then sorted, so the value never changes observable results —
	// it only trades CPU for wall time. RunSource (single in-memory
	// file) ignores this field.
	Concurrency int
	// IntraFileConcurrency caps how many non-NodeChecker rules run
	// concurrently inside one file's checkRules call. The default
	// (0 = auto) computes `max(1, GOMAXPROCS / fileWorkers)` so the
	// inner pool fills whichever cores the outer file-level pool
	// leaves idle: 1 when the file pool saturates cores (mdsmith
	// check on many files), N when the file pool is small (a 5-file
	// PR check on a 16-core host, mdsmith lsp single-file). A
	// caller-set 1 forces serial dispatch; n>1 is taken as the
	// explicit cap. RunSource uses GOMAXPROCS directly because the
	// file-level pool does not run for single-file in-memory paths.
	IntraFileConcurrency int
	// gitignoreCache caches gitignore matchers by directory to avoid
	// re-walking the filesystem for each file. gitignoreMu guards it
	// because Run lints files on multiple goroutines and the
	// GitignoreFunc closure each file carries reaches back into the
	// shared cache lazily during rule execution.
	gitignoreCache map[string]*gitignore.Matcher
	gitignoreMu    sync.Mutex
	// RunCache is the engine-owned read cache shared by every File
	// processed in one Run / RunSource pass. The catalog rule reads
	// through it so a target globbed by N host catalogs is read once
	// per run instead of N times. nil means "create a fresh cache
	// for the next Run" (the CLI case where the corpus is immutable
	// for one process). Callers with a long-lived process — the LSP
	// — install a shared instance so it survives across runLint
	// calls and call its Invalidate seam on document edits.
	RunCache *lint.RunCache
	// BlockOnlyParse, when true, makes lintFile parse only goldmark's
	// block phase (lint.NewFileBlockOnlyPooled) instead of the full
	// parse, so no inline nodes are built. It is a measurement-only
	// flag for the lazy-parse spike (plan 2606141901): the spike
	// benchmark sets it to time "block scan + rules + overhead" as a
	// proxy for a future Layer-0 pipeline. It is default-off and set by
	// no production caller (CLI, LSP, Session), so the shipped lint
	// path is unaffected. Diagnostics from inline-dependent rules are
	// not meaningful under this flag — it exists to measure cost, not
	// to produce correct output.
	BlockOnlyParse bool
	// ParseCache memoizes the parsed *lint.File for an in-memory
	// document so RunSourceWithVersion can skip lint.NewFileFromSource
	// when a prior call at the same (path, version) already parsed
	// the buffer. Opt-in: only the LSP installs one (per-buffer
	// version bumps invalidate stale entries; non-LSP callers that
	// re-use a Runner without a monotonic version key would observe
	// stale results, so leave this nil for them). RunSource (no
	// version) ignores this field and always parses.
	//
	// Pairs with RunCache. populateFileFields stores the per-call
	// RunCache on the parsed *File before publishing it to the cache,
	// so a cache hit reuses that same RunCache pointer. Every Runner
	// that shares one ParseCache MUST also share one RunCache (the
	// LSP wires both off the server). A second Runner with a different
	// RunCache would see the first Runner's cached reads through the
	// hit, and that RunCache's Invalidate seam would be the one
	// driving correctness — easy to get wrong, so don't mix.
	ParseCache *lint.ParseCache
}

// fileOutcome is one file's contribution to a run. Workers fill a
// pre-sized slice of these by index, so the merge is order-stable and
// needs no lock on the shared Result. log holds the file's verbose
// lines (empty unless the logger is enabled); Run flushes them in
// input order so -v output is deterministic regardless of scheduling.
type fileOutcome struct {
	diags []lint.Diagnostic
	errs  []error
	log   []byte
}

// Result holds the output of a lint run.
type Result struct {
	// FilesChecked is the number of files processed (after ignore filtering).
	FilesChecked int
	Diagnostics  []lint.Diagnostic
	Errors       []error
}

// Run lints the files at the given paths and returns a Result containing
// all diagnostics (sorted by file, line, column, message) and any errors
// encountered. Files are linted concurrently (see Runner.Concurrency);
// per-file results are merged in input order before dedupe and sort, so
// the output is identical to a sequential run regardless of scheduling.
func (r *Runner) Run(paths []string) *Result {
	res := &Result{}

	// Resolve the run-scoped read cache for this single pass. A
	// caller-supplied RunCache (the LSP installs one on Server, then
	// invalidates entries on document events) survives across calls;
	// an auto-created one stays local so a Runner re-used for a
	// second Run starts fresh and cannot serve stale reads from the
	// previous corpus. cache is threaded onto every File the per-file
	// loop builds.
	cache := r.runCacheForCall()

	// Run config-target rules once against the config file before per-file
	// markdown processing. These rules (e.g. recipe-safety / MDS040) validate
	// the project config rather than individual Markdown files.
	r.runConfigTargetRules(res)

	work := r.filterIgnored(paths)
	res.FilesChecked = len(work)

	sink := r.log()
	outcomes := r.runFiles(work, cache)
	// Pre-size the merged slice: appending 50+ files' diagnostics
	// through append's geometric growth re-copied the whole set
	// several times on diagnostic-heavy runs.
	total := len(res.Diagnostics)
	for i := range outcomes {
		total += len(outcomes[i].diags)
	}
	if cap(res.Diagnostics) < total {
		merged := make([]lint.Diagnostic, len(res.Diagnostics), total)
		copy(merged, res.Diagnostics)
		res.Diagnostics = merged
	}
	for _, o := range outcomes {
		if len(o.log) > 0 && sink.W != nil {
			_, _ = sink.W.Write(o.log)
		}
		res.Diagnostics = append(res.Diagnostics, o.diags...)
		res.Errors = append(res.Errors, o.errs...)
	}

	// DedupeDiagnostics is only needed when a repo-scoped rule is
	// enabled. Repo-scoped rules (e.g. git-hook-sync / MDS048) anchor
	// their diagnostic to a repository artifact rather than the linted
	// file, so the same tuple can recur across files. When no such rule
	// is enabled, every diagnostic tuple is anchored to its linted file
	// and cannot collide, so the map+slice allocation is pure waste.
	// RunSource (single in-memory file) is exempt: duplicates cannot
	// arise from a single-file lint.
	if r.anyRepoScopedEnabled() {
		res.Diagnostics = lint.DedupeDiagnostics(res.Diagnostics)
	}
	sortDiagnostics(res.Diagnostics)
	return res
}

// filterIgnored drops paths matched by the config ignore list, keeping
// input order.
func (r *Runner) filterIgnored(paths []string) []string {
	work := make([]string, 0, len(paths))
	for _, path := range paths {
		if config.IsIgnored(r.Config.Ignore, path) {
			continue
		}
		work = append(work, path)
	}
	return work
}

// ResolveWorkers maps the Runner.Concurrency knob to an actual worker
// count for a run over n files. concurrency <= 0 means "use
// runtime.GOMAXPROCS"; a positive value is taken literally. The result
// is clamped to n (never more workers than files) and is 0 when there
// is nothing to do.
func ResolveWorkers(concurrency, n int) int {
	if n <= 0 {
		return 0
	}
	w := concurrency
	if w <= 0 {
		w = runtime.GOMAXPROCS(0)
	}
	if w > n {
		w = n
	}
	return w
}

// resolveIntraFileWorkersFor maps the IntraFileConcurrency knob and
// the live file-worker count to an effective concurrency cap for
// non-NodeChecker rules inside one file. The auto path
// (`setting <= 0`) computes `max(1, gomaxproc / max(1, fileWorkers))`
// so the inner pool fills whichever cores the outer pool leaves
// idle. Pulled out as a pure function so the table-test can pin the
// formula without spinning up a Runner.
func resolveIntraFileWorkersFor(setting, gomaxproc, fileWorkers int) int {
	if setting == 1 {
		return 1
	}
	if setting > 1 {
		return setting
	}
	denom := fileWorkers
	if denom <= 0 {
		denom = 1
	}
	n := gomaxproc / denom
	if n < 1 {
		n = 1
	}
	return n
}

// resolveIntraFileWorkers reads GOMAXPROCS once and forwards to the
// pure helper. Callers should use this; the bare helper is pulled out
// as a pure function so a table-driven test can pin the formula
// without faking the runtime.
func resolveIntraFileWorkers(setting, fileWorkers int) int {
	return resolveIntraFileWorkersFor(setting, runtime.GOMAXPROCS(0), fileWorkers)
}

// runFiles lints work into a pre-sized, index-addressed slice. With one
// worker it stays on the calling goroutine. With more, each worker
// clones the rule set once (so rules carrying per-Check state — include's
// visited/chain, the directive engines — never touch another
// goroutine's instance) and pulls file indices off an atomic counter
// for even load balancing. No worker writes a slot another reads, so
// the result needs no lock.
//
// The per-file intra-file concurrency cap (see
// Runner.IntraFileConcurrency) is resolved once here against the
// outer worker count: when the file pool already saturates cores the
// inner cap is 1 (no oversubscription); when only a handful of files
// are linted, the inner cap grows to fill the gap.
//
// cache is the run-scoped read cache resolved by the caller; every
// File built by lintFile receives it so directives in different host
// files share one read per target across the pass.
func (r *Runner) runFiles(work []string, cache *lint.RunCache) []fileOutcome {
	outcomes := make([]fileOutcome, len(work))
	workers := ResolveWorkers(r.Concurrency, len(work))
	intraCap := resolveIntraFileWorkers(r.IntraFileConcurrency, workers)
	// Hoisted out of the per-file loop: the rule→category lookup is a
	// pure function of r.Rules (constant for the run), and the
	// effective-config cache is shared across workers so a uniform
	// config resolves once rather than per file.
	catLookup := ruleCategoryLookup(r.Rules)
	effCache := &effectiveCache{}
	if workers <= 1 {
		rr := runResolve{
			mdRules:   markdownRulesFrom(r.Rules, r.ConfigPath),
			catLookup: catLookup,
			effCache:  effCache,
		}
		for i, path := range work {
			outcomes[i] = r.lintFile(path, intraCap, cache, rr)
		}
		return outcomes
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each worker filters its own rule clones once, not per file.
			rr := runResolve{
				mdRules:   markdownRulesFrom(cloneRules(r.Rules), r.ConfigPath),
				catLookup: catLookup,
				effCache:  effCache,
			}
			for {
				i := int(next.Add(1)) - 1
				if i >= len(work) {
					return
				}
				outcomes[i] = r.lintFile(work[i], intraCap, cache, rr)
			}
		}()
	}
	wg.Wait()
	return outcomes
}

// cloneRules returns an independent copy of rules for one worker so
// concurrent Check calls never share a rule instance's mutable state.
func cloneRules(rules []rule.Rule) []rule.Rule {
	out := make([]rule.Rule, len(rules))
	for i, rl := range rules {
		out[i] = rule.CloneInstance(rl)
	}
	return out
}

// lintFile reads, parses, and checks a single file and returns its
// diagnostics and errors. rr carries the worker's private rule clones,
// the run's rule→category lookup, and the shared effective-config cache
// (see runResolve). It touches no shared Runner state except the
// mutex-guarded gitignore cache and the concurrency-safe rr.effCache.
// intraFileCap controls how many non-NodeChecker rules run concurrently
// for this one file — see runFiles for how the cap is computed from
// Runner.IntraFileConcurrency. cache is installed on the per-File so
// catalog/include rules share one target read across every host file in
// this pass.
func (r *Runner) lintFile(path string, intraFileCap int, cache *lint.RunCache, rr runResolve) (out fileOutcome) {
	// When verbose, log into a per-file buffer instead of the shared
	// logger; Run flushes these in input order so concurrent workers
	// don't interleave -v output. The named return + defer attaches
	// the buffer no matter which early return fires.
	flog := r.log()
	if flog.Enabled {
		var buf bytes.Buffer
		flog = &vlog.Logger{Enabled: true, W: &buf}
		defer func() { out.log = bytes.Clone(buf.Bytes()) }()
	}
	flog.Printf("file: %s", path)

	// Draw a pooled source buffer and read the file into it. The buffer
	// rides the same lifetime boundary as the parse arena: it is returned
	// from the release() closure below, after Check has copied every
	// diagnostic out and the File (its Source/Lines aliasing this buffer)
	// is dead. On the read-error path nothing aliases the buffer yet, so
	// return it immediately.
	bufp := sourceBufPool.Get().(*[]byte)
	source, err := bytelimit.ReadFileLimitedInto(path, bufp, r.MaxInputBytes)
	if err != nil {
		*bufp = (*bufp)[:0]
		sourceBufPool.Put(bufp)
		return fileOutcome{errs: []error{fmt.Errorf("reading %q: %w", path, err)}}
	}

	// The pooled parse recycles AST slab memory across files. lintFile
	// is the documented lifetime boundary: the File and everything
	// aliasing its arena die before the deferred release — diagnostics
	// only carry copied strings and ints, and the RunCache stores
	// Files parsed through its own unpooled path. RunSource (LSP,
	// stdin) deliberately stays on the unpooled constructor because
	// its Files outlive the call via the ParseCache.
	//
	// release() recycles both the arena slabs and the source buffer:
	// the buffer is resliced to zero length so a single large file does
	// not pin its grown capacity in the pool forever.
	f, releaseArena := r.pooledFileConstructor()(path, source, r.StripFrontMatter)
	release := func() {
		releaseArena()
		*bufp = (*bufp)[:0]
		sourceBufPool.Put(bufp)
	}
	defer release()
	f.MaxInputBytes = r.MaxInputBytes
	f.RunCache = cache
	dir := filepath.Dir(path)
	f.FS = os.DirFS(dir)
	gitignoreDir := dir
	if r.RootDir != "" {
		f.SetRootDir(r.RootDir)
		gitignoreDir = r.RootDir
	}
	gd := gitignoreDir // capture for closure
	f.GitignoreFunc = func() *gitignore.Matcher {
		return r.cachedGitignore(gd)
	}

	fmKinds, fmFields, err := r.parseFrontMatter(path, f.FrontMatter)
	if err != nil {
		return fileOutcome{errs: []error{err}}
	}

	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	effective := r.effectiveCached(path, fmKinds, fmFields, rr)
	logRulesTo(flog, rr.mdRules, effective)

	diags, errs := checkRulesWithIntraFile(f, rr.mdRules, effective, r.SkipSourceContext, intraFileCap)
	if r.Explain {
		explain.Attach(diags, r.Config, path, fmKinds, fmFields)
	}
	return fileOutcome{diags: diags, errs: errs}
}

// pooledFileConstructor returns the per-file parse constructor for this
// run: the full-parse pooled constructor normally, or the block-only one
// when the lazy-parse spike flag (Runner.BlockOnlyParse) is set. The flag
// is default-off and no production caller sets it, so the returned
// constructor is lint.NewFileFromSourcePooled on every shipped path.
func (r *Runner) pooledFileConstructor() func(string, []byte, bool) (*lint.File, func()) {
	if r.BlockOnlyParse {
		return lint.NewFileBlockOnlyPooled
	}
	return lint.NewFileFromSourcePooled
}

// RunSource lints in-memory source bytes (e.g. from stdin or an LSP
// buffer) and returns a Result. It creates a File via
// NewFileFromSource, determines the effective config, and uses
// CheckRules (which includes clone+settings logic and line-offset
// adjustment).
//
// When Runner.SourceFS is non-nil, RunSource wires it onto the File
// as f.FS so include/catalog/cross-file rules see the same
// filesystem view processFile sets up for on-disk runs.
//
// f.GitignoreFunc is wired against a directory chosen in this order:
//
//  1. Runner.RootDir, when set (matches processFile's anchoring).
//  2. filepath.Dir(path), when path is absolute and RootDir is empty.
//
// With a relative path and no RootDir (the bare `<stdin>` case),
// GitignoreFunc stays nil — the matcher would have no meaningful
// root to walk anyway. FS-aware rules still see SourceFS regardless.
//
// When SourceFS is nil (the stdin case), FS stays nil and rules that
// require it short-circuit just as they did before.
func (r *Runner) RunSource(path string, source []byte) *Result {
	// noParseCacheVersion routes RunSource through the shared body
	// with a sentinel that suppresses ParseCache use. RunSource has
	// no version coordinate and cannot key the cache safely, so even
	// when r.ParseCache is non-nil this path stays cold.
	return r.runSource(path, source, 0, false)
}

// RunSourceWithVersion is the LSP-facing entry point. It mirrors
// RunSource but accepts the textDocument/version coordinate the
// editor maintains for the buffer. When r.ParseCache is installed,
// it serves a *lint.File from the cache at (path, version) and skips
// lint.NewFileFromSource; otherwise it parses fresh and stores the
// result for the next call.
//
// Non-LSP callers that already use RunSource keep their existing
// signature. New callers that have a version coordinate but no
// cache installed can pass version anyway — it is ignored when
// r.ParseCache is nil.
func (r *Runner) RunSourceWithVersion(path string, source []byte, version int) *Result {
	return r.runSource(path, source, version, true)
}

// runSource carries the shared RunSource / RunSourceWithVersion
// body. useParseCache gates the ParseCache lookup so the version-
// less RunSource entry cannot accidentally serve a *File parsed for
// some other LSP edit cycle.
func (r *Runner) runSource(path string, source []byte, version int, useParseCache bool) *Result {
	res := &Result{FilesChecked: 1}

	// Run config-target rules once before processing the in-memory source,
	// matching the behavior of Run() so config diagnostics surface via stdin.
	// This must happen before the size guard so an oversized buffer cannot
	// hide config-level errors that Run() would have surfaced regardless of
	// any individual file's size.
	r.runConfigTargetRules(res)

	// Mirror the on-disk size cap that bytelimit.ReadFileLimited /
	// readStdinLimited apply to file and stdin reads. Without this
	// guard, in-memory callers (LSP, other integrations) would parse
	// arbitrarily large buffers and diverge from `mdsmith check`'s
	// "file too large" failure mode.
	if r.MaxInputBytes > 0 && r.MaxInputBytes != math.MaxInt64 &&
		int64(len(source)) > r.MaxInputBytes {
		// Match the on-disk error shape — processFile wraps
		// bytelimit.ReadFileLimited's "file too large" via
		// `reading %q: %w`, so editor / log output stays
		// uniform whether the source came from stdin, an LSP
		// buffer, or a real file on disk.
		res.Errors = append(res.Errors,
			fmt.Errorf("reading %q: file too large (%d bytes, max %d)",
				path, len(source), r.MaxInputBytes))
		return res
	}

	r.log().Printf("file: %s", path)

	f, err := r.parseForSource(path, source, version, useParseCache)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
		return res
	}

	fmKinds, fmFields, err := r.parseFrontMatter(path, f.FrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, err)
		return res
	}

	r.runSourceCheckRules(res, f, path, fmKinds, fmFields)
	sortDiagnostics(res.Diagnostics)
	return res
}

// parseForSource resolves the *lint.File for an in-memory source.
// When useParseCache is true and r.ParseCache holds an entry at
// (path, version), the cached *File is returned and the parse is
// skipped. Otherwise lint.NewFileFromSource is called, the Runner-
// derived fields (MaxInputBytes, RunCache, FS, RootDir, gitignore
// hook, generated-section ranges) are populated, and the result is
// stored at (path, version) for the next call.
//
// Populating those fields before the Put keeps the cached *File
// effectively immutable from each caller's perspective: a concurrent
// Get returns a *File whose per-call state was set exactly once by
// the goroutine that did the parse, so two LSP requests for the same
// (path, version) cannot race on field writes. The stale-Put
// rejection inside ParseCache.Put keeps an older parse landing late
// from clobbering a newer cached value or re-filling a just-cleared
// slot.
func (r *Runner) parseForSource(path string, source []byte, version int, useParseCache bool) (*lint.File, error) {
	if useParseCache && r.ParseCache != nil {
		if f, ok := r.ParseCache.Get(path, version); ok {
			return f, nil
		}
	}
	f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
	if err == nil {
		r.populateFileFields(f, path)
		if useParseCache && r.ParseCache != nil {
			r.ParseCache.Put(path, version, f)
		}
	}
	return f, err
}

// populateFileFields sets the Runner-derived state on f that
// downstream checks rely on: MaxInputBytes, RunCache, FS, RootDir,
// the lazy gitignore hook, and the generated-section ranges.
// Factored out of runSource so parseForSource can call it once
// before the *File is published to the parse cache — see that
// method's comment for the racing-readers argument.
func (r *Runner) populateFileFields(f *lint.File, path string) {
	f.MaxInputBytes = r.MaxInputBytes
	f.RunCache = r.runCacheForCall()
	if r.SourceFS != nil {
		f.FS = r.SourceFS
	}
	// Mirror processFile's gitignore wiring so on-disk Run() and
	// in-memory RunSource() agree on whether a path is ignored.
	// Anchor at RootDir when set, otherwise fall back to the
	// document directory (filepath.Dir(path)) when path is absolute.
	// In-memory callers with neither RootDir nor an absolute path
	// (the bare `<stdin>` case) leave GitignoreFunc nil — the
	// matcher would have no meaningful root to walk anyway.
	gitignoreDir := ""
	switch {
	case r.RootDir != "":
		f.SetRootDir(r.RootDir)
		gitignoreDir = r.RootDir
	case filepath.IsAbs(path):
		gitignoreDir = filepath.Dir(path)
	}
	// An in-memory workspace (the Session's MemWorkspace, e.g. the WASM
	// build) has no on-disk RootDir, but its SourceFS is rooted at the
	// project root — the same contract os.DirFS(RootDir) gives on disk.
	// Wire it as RootFS so RootFS-aware cross-file rules (include's ".."
	// resolution, MDS020's schema reads) read through the workspace
	// instead of falling back to os.*, which is "not implemented on js".
	if r.RootDir == "" && r.SourceFS != nil {
		f.RootFS = r.SourceFS
	}
	if gitignoreDir != "" {
		gd := gitignoreDir
		f.GitignoreFunc = func() *gitignore.Matcher {
			return r.cachedGitignore(gd)
		}
	}
	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)
}

// runSourceCheckRules wraps the post-parse check pipeline for
// RunSource: resolve the intra-file concurrency cap, run the rule
// set, attach explanation provenance, and append diagnostics to res.
// Split out so RunSource itself stays under the funlen cap.
func (r *Runner) runSourceCheckRules(
	res *Result, f *lint.File, path string,
	fmKinds []string, fmFields map[string]any,
) {
	// f.GeneratedRanges and all other Runner-derived fields are now
	// populated once by parseForSource before the cache Put, so this
	// path no longer mutates f and concurrent cache hits do not race.
	effective := r.effectiveWithCategories(path, fmKinds, fmFields)
	mdRules := r.markdownRules()
	r.logRules(mdRules, effective)

	// RunSource has no file-level pool to compete with, so the
	// intra-file cap defaults to GOMAXPROCS (auto = pass 0 file
	// workers, formula picks the full host). The explicit
	// IntraFileConcurrency knob still overrides — set 1 to keep
	// the LSP single-threaded for predictability.
	intraFileCap := resolveIntraFileWorkers(r.IntraFileConcurrency, 0)
	diags, errs := checkRulesWithIntraFile(f, mdRules, effective, r.SkipSourceContext, intraFileCap)
	if r.Explain {
		explain.Attach(diags, r.Config, path, fmKinds, fmFields)
	}
	res.Diagnostics = append(res.Diagnostics, diags...)
	res.Errors = append(res.Errors, errs...)
}

// markdownRules returns the subset of rules to run against individual Markdown
// files. When ConfigPath is set, config-target rules are excluded because they
// have already run once (via runConfigTargetRules) and their Check method
// returns nil for any non-config path anyway.
func (r *Runner) markdownRules() []rule.Rule {
	return markdownRulesFrom(r.Rules, r.ConfigPath)
}

// markdownRulesFrom filters config-target rules out of rules when a
// config path is set (they ran once via runConfigTargetRules and their
// Check returns nil for non-config paths anyway). It operates on the
// passed slice so each worker filters its own clones.
func markdownRulesFrom(rules []rule.Rule, configPath string) []rule.Rule {
	if configPath == "" {
		return rules
	}
	filtered := make([]rule.Rule, 0, len(rules))
	for _, rl := range rules {
		if ct, ok := rl.(rule.ConfigTarget); ok && ct.IsConfigFileRule() {
			continue
		}
		filtered = append(filtered, rl)
	}
	return filtered
}

// parseFrontMatterKinds parses and validates the kinds list from a file's
// front-matter block, returning a combined error on parse or validation failure.
func (r *Runner) parseFrontMatterKinds(path string, fm []byte) ([]string, error) {
	kinds, err := lint.ParseFrontMatterKinds(fm)
	if err != nil {
		return nil, fmt.Errorf("parsing front-matter kinds in %q: %w", path, err)
	}
	if err := config.ValidateFrontMatterKinds(r.Config, path, kinds); err != nil {
		return nil, err
	}
	return kinds, nil
}

// parseFrontMatterFields parses a file's front-matter block into a
// top-level map. It feeds the kind-assignment `fields-present:` selector
// and returns (nil, nil) when no entry could match this file — skipping
// the full YAML decode for files no fields-present entry would ever
// claim. Files outside every fields-present glob keep the kinds-only
// parse path (and its narrower error surface).
func (r *Runner) parseFrontMatterFields(path string, fm []byte) (map[string]any, error) {
	if !config.NeedsFieldsForFile(r.Config, path) {
		return nil, nil
	}
	fields, err := lint.ParseFrontMatterFields(fm)
	if err != nil {
		return nil, fmt.Errorf("parsing front matter in %q: %w", path, err)
	}
	return fields, nil
}

// parseFrontMatter is the shared kinds+fields parse used by both Run and
// RunSource; pulling it out keeps each entry point under the funlen cap.
func (r *Runner) parseFrontMatter(path string, fm []byte) ([]string, map[string]any, error) {
	kinds, err := r.parseFrontMatterKinds(path, fm)
	if err != nil {
		return nil, nil, err
	}
	fields, err := r.parseFrontMatterFields(path, fm)
	if err != nil {
		return nil, nil, err
	}
	return kinds, fields, nil
}

// effectiveWithCategories computes the effective rule config for a file
// path, applying category-based enable/disable on top of per-rule settings.
func (r *Runner) effectiveWithCategories(
	path string, fmKinds []string, fmFields map[string]any,
) map[string]config.RuleCfg {
	effective, categories, explicit := config.EffectiveAll(r.Config, path, fmKinds, fmFields)
	return config.ApplyCategories(effective, categories, ruleCategoryLookup(r.Rules), explicit)
}

// effectiveCache memoizes effective rule configs by config signature for
// the span of one runFiles call. File workers read and write it
// concurrently, so it wraps sync.Map. The stored maps are read-only
// downstream — classifyRules clones a rule's settings before applying
// them — so sharing one map across files is safe.
type effectiveCache struct{ m sync.Map }

func (c *effectiveCache) get(key string) (map[string]config.RuleCfg, bool) {
	v, ok := c.m.Load(key)
	if !ok {
		return nil, false
	}
	return v.(map[string]config.RuleCfg), true
}

// loadOrStore returns the cached map for key, storing and returning v
// when none exists yet. Concurrent misses on the same key thus converge
// on one shared instance instead of each keeping its own equal copy.
func (c *effectiveCache) loadOrStore(key string, v map[string]config.RuleCfg) map[string]config.RuleCfg {
	actual, _ := c.m.LoadOrStore(key, v)
	return actual.(map[string]config.RuleCfg)
}

// runResolve bundles the resolution state lintFile needs, across two
// lifetimes: mdRules is per-worker (each worker filters its own rule
// clones), while catLookup and effCache are per-run — built once and
// shared across all workers (effCache is concurrency-safe). Bundling
// keeps lintFile's signature small and hoists work the per-file loop
// used to repeat.
type runResolve struct {
	mdRules   []rule.Rule
	catLookup func(string) string
	effCache  *effectiveCache
}

// effectiveCached is the hot-path config resolver: it memoizes the
// effective rule config on rr.effCache keyed by the file's config
// signature and reuses the prebuilt rr.catLookup, so a corpus that
// shares one config resolves it once instead of per file. Cold callers
// (RunSource, config-target, defaults) keep using effectiveWithCategories.
func (r *Runner) effectiveCached(
	path string, fmKinds []string, fmFields map[string]any, rr runResolve,
) map[string]config.RuleCfg {
	key, kinds := config.EffectiveSignature(r.Config, path, fmKinds, fmFields)
	if v, ok := rr.effCache.get(key); ok {
		return v
	}
	effective, categories, explicit := config.EffectiveAllForKinds(r.Config, path, kinds)
	res := config.ApplyCategories(effective, categories, rr.catLookup, explicit)
	return rr.effCache.loadOrStore(key, res)
}

// runCacheForCall returns the run-scoped read cache to thread through
// the next Run / RunSource pass. A caller-supplied r.RunCache (the
// LSP installs one on Server and invalidates entries on document
// events) is reused as-is. When nil, a fresh cache is built for this
// call only and never stored on r — a Runner re-used for a second
// Run starts clean and cannot serve stale reads from the previous
// corpus.
func (r *Runner) runCacheForCall() *lint.RunCache {
	if r.RunCache != nil {
		return r.RunCache
	}
	return lint.NewRunCache()
}

// cachedGitignore returns a *gitignore.Matcher for the given directory,
// creating and caching it on first use to avoid re-walking the filesystem.
// The cache key is normalized to an absolute path so equivalent paths
// (e.g. "sub" vs "./sub") share the same entry.
func (r *Runner) cachedGitignore(dir string) *gitignore.Matcher {
	r.gitignoreMu.Lock()
	defer r.gitignoreMu.Unlock()
	if r.gitignoreCache == nil {
		r.gitignoreCache = make(map[string]*gitignore.Matcher)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = filepath.Clean(dir)
	}
	if m, ok := r.gitignoreCache[absDir]; ok {
		return m
	}
	m := gitignore.NewMatcher(absDir)
	r.gitignoreCache[absDir] = m
	return m
}

// log returns the runner's logger. If no logger is set, it returns a
// disabled logger so callers don't need nil checks.
func (r *Runner) log() *vlog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return &vlog.Logger{}
}

// logRules logs each enabled rule in the effective config from the provided slice.
func (r *Runner) logRules(rules []rule.Rule, effective map[string]config.RuleCfg) {
	logRulesTo(r.log(), rules, effective)
}

// logRulesTo logs each enabled rule to l. Split from logRules so the
// per-file buffered logger in lintFile can reuse the same formatting
// without going through the shared Runner logger.
func logRulesTo(l *vlog.Logger, rules []rule.Rule, effective map[string]config.RuleCfg) {
	if !l.Enabled {
		return
	}
	for _, rl := range rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		l.Printf("rule: %s %s", rl.ID(), rl.Name())
	}
}

// ruleCategoryLookup returns a function that maps a rule name to its category.
func ruleCategoryLookup(rules []rule.Rule) func(string) string {
	m := make(map[string]string, len(rules))
	for _, rl := range rules {
		m[rl.Name()] = rl.Category()
	}
	return func(name string) string {
		return m[name]
	}
}

// runConfigTargetRules runs rules that implement rule.ConfigTarget once
// against a synthetic lint.File for the config file. These rules validate
// the project config rather than individual Markdown files. They are skipped
// in the normal per-file loop because their Check method returns nil for
// non-config file paths.
func (r *Runner) runConfigTargetRules(res *Result) {
	if r.ConfigPath == "" {
		return
	}
	effective := r.effectiveWithCategories(r.ConfigPath, nil, nil)
	f, err := lint.NewFile(r.ConfigPath, []byte{})
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("creating config lint.File: %w", err))
		return
	}
	for _, rl := range r.Rules {
		configTarget, ok := rl.(rule.ConfigTarget)
		if !ok || !configTarget.IsConfigFileRule() {
			continue
		}
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		configured, err := checker.ConfigureRule(rl, cfg)
		if err != nil {
			res.Errors = append(res.Errors, err)
			continue
		}
		diags := configured.Check(f)
		res.Diagnostics = append(res.Diagnostics, diags...)
	}
}

// anyRepoScopedEnabled reports whether any markdown rule (excluding
// ConfigTarget rules) implements rule.RepoScoped and is enabled in the
// global effective configuration. Run uses this once to decide whether
// DedupeDiagnostics is needed: when no enabled rule is repo-scoped,
// every diagnostic tuple is anchored to its linted file and cross-file
// duplicates cannot occur, so the map+slice allocation is skipped.
//
// The effective config is queried with an empty path and nil front-matter
// so kind-specific overrides do not influence the result. A repo-scoped
// rule enabled only for a specific kind is conservatively treated as
// potentially enabled (it is still surfaced by its global config entry).
//
// RunSource is a single in-memory file and is exempt from this check:
// a single-file lint cannot produce cross-file duplicates.
func (r *Runner) anyRepoScopedEnabled() bool {
	mdRules := markdownRulesFrom(r.Rules, r.ConfigPath)
	effective := r.effectiveWithCategories("", nil, nil)
	for _, rl := range mdRules {
		rs, ok := rl.(rule.RepoScoped)
		if !ok || !rs.RepoScopedDiagnostics() {
			continue
		}
		cfg, ok := effective[rl.Name()]
		if ok && cfg.Enabled {
			return true
		}
	}
	return false
}

// sortDiagnostics sorts diagnostics by file, line, column, then message.
// sort.SliceStable preserves the input order only for diagnostics that are
// equal on all compared fields, including Message.
func sortDiagnostics(diags []lint.Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		di, dj := diags[i], diags[j]
		if di.File != dj.File {
			return di.File < dj.File
		}
		if di.Line != dj.Line {
			return di.Line < dj.Line
		}
		if di.Column != dj.Column {
			return di.Column < dj.Column
		}
		return di.Message < dj.Message
	})
}
