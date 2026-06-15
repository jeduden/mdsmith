package mdsmith

import (
	"os"

	"github.com/jeduden/mdsmith/internal/engine"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	vlog "github.com/jeduden/mdsmith/internal/log"
)

// BatchOptions tunes a multi-file batch operation ([Session.CheckPaths],
// [Session.FixPaths]). All fields are optional; the zero value runs at
// the engine defaults (GOMAXPROCS concurrency, no explain, no verbose
// logging, the session's configured byte cap).
//
// Batch ops are a native power surface: they read and (for FixPaths)
// write many files on disk, walk in parallel, and return the engine's
// own Result/fix.Result so the CLI keeps its discovery, ordering, and
// output formatting. They have no JavaScript mirror — a WASM host has no
// disk and drives single files through [Session.Check] / [Session.Fix].
// See docs/background/concepts/engine-api.md.
type BatchOptions struct {
	// Explain, when true, attaches per-leaf rule provenance to each
	// diagnostic (the CLI's --explain flag).
	Explain bool
	// Concurrency caps how many files are linted/fixed in parallel.
	// Zero or negative means "use runtime.GOMAXPROCS"; the engine
	// clamps the worker count to the file count. The value never
	// changes observable output — only CPU vs wall-time.
	Concurrency int
	// MaxInputBytes overrides the session's default byte cap for this
	// call. Zero leaves the session default in place; a positive value
	// rejects files larger than that many bytes; a negative value
	// (or math.MaxInt64) means unlimited. The CLI passes the value it
	// resolved from --max-input-size / config here.
	MaxInputBytes int64
	// Logger receives the engine's verbose trace lines (config, files,
	// rules) for the CLI's -v flag. Nil means no verbose output.
	Logger *vlog.Logger
	// DryRun applies only to [Session.FixPaths]: run the full fix
	// pipeline but write nothing back to disk, recording the per-rule
	// would-fix tally on the result instead (the CLI's `fix --dry-run`).
	// Ignored by CheckPaths.
	DryRun bool
}

// CheckPaths lints the on-disk files at paths and returns the aggregate
// engine result — diagnostics sorted by file, line, column, message,
// plus any per-file errors and the count of files checked. It drives
// engine.Runner.Run internally with the session's compiled config,
// rule set, root, and config path, so config-target rules (MDS040) run
// once for the whole run rather than once per file.
//
// CheckPaths is native-only: it reads files from disk through the
// engine's path-based loop. A WASM host lints in-memory buffers through
// [Session.Check] instead. The caller (cmd/mdsmith) keeps file
// discovery, ignore filtering at the walk layer, and output formatting.
func (s *Session) CheckPaths(paths []string, opts BatchOptions) *engine.Result {
	return s.newBatchRunner(opts).Run(paths)
}

// FixPaths auto-fixes the on-disk files at paths in place and returns
// the aggregate fix result — remaining diagnostics, the list of
// modified files, the would-fix preview (when DryRun is set), and any
// errors. It drives fix.Fixer.Fix internally with the session's compiled
// config, rule set, root, and the per-call BatchOptions.
//
// FixPaths is native-only: the fixer reads and writes files on disk. A
// WASM host fixes in-memory buffers through [Session.Fix] instead. The
// caller (cmd/mdsmith) keeps file discovery, leaves-first ordering, and
// output formatting; pass paths already ordered so a cross-file
// generated-section cascade converges in one productive pass.
func (s *Session) FixPaths(paths []string, opts BatchOptions) *fixpkg.Result {
	maxBytes := s.maxBytes
	if opts.MaxInputBytes != 0 {
		maxBytes = opts.MaxInputBytes
	}
	fixer := &fixpkg.Fixer{
		Config:           s.cfg,
		Rules:            s.rules,
		StripFrontMatter: frontMatterEnabled(s.cfg),
		Logger:           opts.Logger,
		RootDir:          s.rootDir,
		MaxInputBytes:    maxBytes,
		Explain:          opts.Explain,
		DryRun:           opts.DryRun,
	}
	return fixer.Fix(paths)
}

// CheckSource lints one in-memory source (e.g. stdin) and returns the
// engine result, including any config-target rule findings against the
// loaded config. It is the single-source sibling of CheckPaths: the CLI
// uses it for `mdsmith check -` so stdin shares the session path, while
// the public [Session.Check] serves the cached, JS-mirrored single-file
// surface. uri is the display path ("<stdin>"); cross-file rules see no
// SourceFS, matching the engine's stdin behaviour.
//
// Native-only: it returns the engine's own Result so the CLI reuses its
// reporting. A WASM host uses [Session.Check] (which returns the public
// Diagnostic shape) instead.
func (s *Session) CheckSource(uri string, source []byte, opts BatchOptions) *engine.Result {
	return s.newBatchRunner(opts).RunSource(uri, source)
}

// newBatchRunner builds the engine.Runner shared by CheckPaths (and, on
// the path-based fixer side, the post-fix lint). It threads the session
// state plus the per-call BatchOptions. MaxInputBytes falls back to the
// session default when the option is zero.
func (s *Session) newBatchRunner(opts BatchOptions) *engine.Runner {
	maxBytes := s.maxBytes
	if opts.MaxInputBytes != 0 {
		maxBytes = opts.MaxInputBytes
	}
	return &engine.Runner{
		Config:           s.cfg,
		Rules:            s.rules,
		StripFrontMatter: frontMatterEnabled(s.cfg),
		Logger:           opts.Logger,
		RootDir:          s.rootDir,
		MaxInputBytes:    maxBytes,
		Explain:          opts.Explain,
		Concurrency:      opts.Concurrency,
		ConfigPath:       s.cfgPath,
		// Lazy-parse spike (plan 2606141901) measurement seam: when the
		// MDSMITH_SPIKE_BLOCK_ONLY environment variable is set, parse only
		// goldmark's block phase so hyperfine can time "block scan + rules
		// + overhead" against gomarklint in the real CLI. Off by default —
		// an unset variable leaves the shipped behaviour byte-identical —
		// and undocumented. Diagnostics under this flag are not correct
		// (inline rules see no inline nodes); it exists to measure cost.
		BlockOnlyParse: os.Getenv("MDSMITH_SPIKE_BLOCK_ONLY") != "",
		// Flat Layer-0 prototype (plan 2606142147) measurement seam: when
		// MDSMITH_SPIKE_FLAT_L0 is set, an eligible run (line-capable rules
		// only) skips the goldmark parse and drives line rules from the
		// flat line classifier, so hyperfine can time the parse-free path
		// against gomarklint. Off by default and undocumented; on an
		// ineligible config it is inert (the engine keeps the AST path),
		// so diagnostics stay correct.
		FlatLayer0: os.Getenv("MDSMITH_SPIKE_FLAT_L0") != "",
	}
}
