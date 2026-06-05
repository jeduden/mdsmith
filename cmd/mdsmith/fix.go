package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// runFix implements the "fix" subcommand: auto-fix lint issues in place.
func runFix(args []string) int {
	opts, fileArgs, hasStdin, code := parseFixFlags(args)
	if code >= 0 {
		return code
	}
	tuneGCForBatch()
	if hasStdin {
		fmt.Fprintf(os.Stderr, "mdsmith: cannot fix stdin in place\n")
		return 2
	}
	if len(fileArgs) > 0 {
		return fixFiles(fileArgs, opts)
	}
	// No file args: discover files from config.
	return fixDiscovered(opts)
}

// parseFixFlags configures the `fix` flag set, parses args, and
// returns the resolved opts plus positional arguments. The bool
// `hasStdin` is true when the caller passed `-` as a positional
// arg. A non-negative `code` means the caller should return that
// exit code immediately (e.g. --help or a parse error).
func parseFixFlags(args []string) (fixCLIOpts, []string, bool, int) {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	var (
		configPath, format, maxInputSize                                      string
		noColor, quiet, verbose, noGitignore, followSymlinks, explain, dryRun bool
	)

	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	fs.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	fs.BoolVarP(&verbose, "verbose", "v", false, "Show config, files, and rules on stderr")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&maxInputSize, "max-input-size", "", "Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")
	fs.BoolVar(&explain, "explain", false, "Attach per-leaf rule provenance to each remaining diagnostic")
	fs.BoolVar(&dryRun, "dry-run", false,
		"Preview which files would change without writing; "+
			"per-file output lists the rules that would fire and their counts")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith fix [flags] [files...]\n\n"+
			"Auto-fix lint issues in Markdown files.\n\n"+
			"Files can be paths, directories (walked recursively for *.md), or glob patterns.\n"+
			"Pass - to read from stdin (rejected: files must be writable).\n"+
			"With no file arguments, discovers files using config patterns.\n\n"+
			"Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: fix"); code >= 0 {
			return fixCLIOpts{}, nil, false, code
		}
	}

	// --quiet suppresses verbose
	if quiet {
		verbose = false
	}

	hasStdin, fileArgs := splitStdinArg(fs.Args())

	return fixCLIOpts{
		configPath: configPath,
		format:     format,
		noColor:    noColor,
		quiet:      quiet,
		verbose:    verbose,
		walk: walkCLI{
			noGitignore:    noGitignore,
			followSymlinks: followSymlinksOverride(fs, followSymlinks),
		},
		maxInputSize: maxInputSize,
		explain:      explain,
		dryRun:       dryRun,
	}, fileArgs, hasStdin, -1
}

// fixCLIOpts bundles the runtime knobs threaded through the fix
// command path. Grouped because runFix splits between explicit-file
// and config-discovery entry points and the same nine values flow
// to both.
type fixCLIOpts struct {
	configPath   string
	format       string
	noColor      bool
	quiet        bool
	verbose      bool
	walk         walkCLI
	maxInputSize string
	explain      bool
	dryRun       bool
}

// fixFiles fixes lint issues in the given file paths.
func fixFiles(fileArgs []string, opts fixCLIOpts) int {
	cfg, cfgPath, logger, files, maxBytes, code := loadAndResolve(
		fileArgs, opts.configPath, opts.verbose, opts.walk, opts.maxInputSize,
	)
	if code >= 0 {
		return code
	}
	return runFixThroughSession(cfg, cfgPath, opts, logger, files, maxBytes)
}

// fixDiscovered loads config, discovers files from config patterns,
// and fixes them. Returns the appropriate exit code.
func fixDiscovered(opts fixCLIOpts) int {
	cfg, cfgPath, logger, files, code := discoverFiles(opts.configPath, opts.verbose, opts.walk)
	if code >= 0 {
		return code
	}

	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return runFixThroughSession(cfg, cfgPath, opts, logger, files, maxBytes)
}

// runFixThroughSession orders files leaves-first, builds a Session over
// the already-loaded config, and runs Session.FixPaths — the shared body
// of fixFiles and fixDiscovered. Leaves-first ordering stays here in the
// CLI (a convergence optimisation over the engine's fixpoint loop), so
// FixPaths receives an already-ordered list.
func runFixThroughSession(
	cfg *config.Config, cfgPath string, opts fixCLIOpts,
	logger *vlog.Logger, files []string, maxBytes int64,
) int {
	files = orderFilesLeavesFirst(files, rootDirFromConfig(cfgPath), maxBytes)
	sess := sessionForCLI(cfg, cfgPath)
	defer sess.Dispose()
	fixResult := sess.FixPaths(files, mdsmith.BatchOptions{
		Explain:       opts.explain,
		MaxInputBytes: batchMaxBytes(maxBytes),
		Logger:        logger,
		DryRun:        opts.dryRun,
	})
	return reportFixResult(opts, fixResult, logger)
}

// orderFilesLeavesFirst reorders files so generated-section
// dependencies are fixed before the files that embed them, letting a
// single `mdsmith fix` pass converge an include/catalog cascade
// instead of needing one pass per dependency level. It builds the
// workspace dependency index over files and topologically sorts it
// (leaves first; see index.DependencyOrder).
//
// Ordering is a convergence optimization, not a correctness
// requirement — Fixer.Fix re-sweeps to a fixpoint regardless — so any
// setup problem (a short list, two inputs that collapse to one
// workspace path) falls back to the caller's original order rather
// than failing or dropping a file.
func orderFilesLeavesFirst(files []string, rootDir string, maxBytes int64) []string {
	if len(files) < 2 {
		return files
	}
	relToAbs := make(map[string]string, len(files))
	rels := make([]string, 0, len(files))
	for _, abs := range files {
		rel := index.NormalizePath(workspaceRelativePath(abs, rootDir))
		if _, dup := relToAbs[rel]; dup {
			return files
		}
		relToAbs[rel] = abs
		rels = append(rels, rel)
	}

	idx := index.New(rootDir)
	idx.BuildSerial(rels, func(rel string) ([]byte, error) {
		return bytelimit.ReadFileLimited(relToAbs[rel], maxBytes)
	})

	ordered := idx.DependencyOrder(rels)
	out := make([]string, 0, len(files))
	for _, rel := range ordered {
		out = append(out, relToAbs[rel])
	}
	return out
}

// reportFixResult writes the fix run's output and computes the exit
// code. Shared by fixFiles and fixDiscovered so the dry-run preview,
// stats summary, and exit-code logic stay in one place.
func reportFixResult(opts fixCLIOpts, fixResult *fixpkg.Result, logger *vlog.Logger) int {
	return reportFixResultTo(opts, fixResult, logger, os.Stderr)
}

// reportFixResultTo is the injectable form of reportFixResult. Tests
// pass an alternate stderr writer to exercise the write-error
// branches without leaking to the real stderr; the formatter,
// writeDryRunJSON, and the run-stats helper all route their own
// write-error messages through the same writer so a fault-injecting
// writer captures the full stderr surface.
func reportFixResultTo(opts fixCLIOpts, fixResult *fixpkg.Result, logger *vlog.Logger, stderrW io.Writer) int {
	printErrorsTo(stderrW, fixResult.Errors)

	if opts.dryRun && opts.format == "json" && !opts.quiet {
		// Match `check --format json` and `fix --format json`: lint
		// output goes to stderr (see docs/reference/cli.md "Output").
		if code := writeDryRunJSON(stderrW, fixResult); code != 0 {
			return code
		}
	} else {
		if opts.dryRun && !opts.quiet {
			printDryRunPreview(stderrW, fixResult)
		}
		if !opts.quiet && len(fixResult.Diagnostics) > 0 {
			if code := formatDiagnosticsTo(stderrW, fixResult.Diagnostics, opts.format, opts.noColor); code != 0 {
				return code
			}
		}
	}

	printRunStatsTo(stderrW, opts.format, opts.quiet, runStats{
		Checked:  fixResult.FilesChecked,
		Fixed:    len(fixResult.Modified),
		Failures: fixResult.Failures,
		Unfixed:  len(fixResult.Diagnostics),
		WouldFix: fixResult.WouldFix,
		DryRun:   opts.dryRun,
	})
	logger.Printf("checked %d files, %d issues found", fixResult.FilesChecked, len(fixResult.Diagnostics))

	if len(fixResult.Errors) > 0 && len(fixResult.Diagnostics) == 0 {
		return 2
	}
	if len(fixResult.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// printDryRunPreview writes one line per file that would change.
// Files where some diagnostics would be resolved get a
// "would fix N violations (MDS001 ×2, MDS006)" line; files whose
// bytes would change without any diagnostic count decreasing get a
// "would update generated content" line so a dry-run still surfaces
// directive regeneration. No-op when the preview is empty. Write
// errors on the destination are intentionally ignored: the preview
// is supplemental context, not the primary signal (exit code and
// diagnostic output drive CI), and a half-written preview is not
// worth halting the run for.
func printDryRunPreview(w io.Writer, fixResult *fixpkg.Result) {
	for _, wf := range fixResult.WouldFixFiles {
		if wf.Count == 0 {
			_, _ = fmt.Fprintf(w, "%s: would update generated content\n", wf.Path)
			continue
		}
		_, _ = fmt.Fprintf(w, "%s: would fix %s\n", wf.Path, formatWouldFixSummary(wf))
	}
}

// formatWouldFixSummary renders "N violations (MDS001 ×2, MDS006)"
// for one file's would-fix entry. Single-count rules are printed
// without a multiplier so the common case stays compact.
func formatWouldFixSummary(wf fixpkg.WouldFixFile) string {
	parts := make([]string, 0, len(wf.Rules))
	for _, r := range wf.Rules {
		if r.Count == 1 {
			parts = append(parts, r.RuleID)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s ×%d", r.RuleID, r.Count))
	}
	noun := "violations"
	if wf.Count == 1 {
		noun = "violation"
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d %s", wf.Count, noun)
	}
	return fmt.Sprintf("%d %s (%s)", wf.Count, noun, strings.Join(parts, ", "))
}

// dryRunJSONFile is the per-file JSON shape produced by
// `mdsmith fix --dry-run --format json`. One record per file
// whose bytes or diagnostic counts would change — fixable
// violations, remaining unfixable diagnostics, or
// generated-section regeneration (would_fix=0, rules=[]).
type dryRunJSONFile struct {
	Path        string     `json:"path"`
	WouldFix    int        `json:"would_fix"`
	Rules       []string   `json:"rules"`
	Diagnostics []jsonDiag `json:"diagnostics"`
}

// jsonDiag mirrors the diagnostic shape JSONFormatter writes today
// so dry-run JSON records carry the same per-diagnostic fields,
// including the optional source-context and explanation trailers.
type jsonDiag struct {
	File            string       `json:"file"`
	Line            int          `json:"line"`
	Column          int          `json:"column"`
	Rule            string       `json:"rule"`
	Name            string       `json:"name"`
	Severity        string       `json:"severity"`
	Message         string       `json:"message"`
	SourceLines     []string     `json:"source_lines,omitempty"`
	SourceStartLine int          `json:"source_start_line,omitempty"`
	Explanation     *jsonDiagExp `json:"explanation,omitempty"`
	Deprecated      bool         `json:"deprecated,omitempty"`
	ReplacedBy      string       `json:"replaced_by,omitempty"`
}

// jsonDiagExp is the explanation trailer shape, matching
// output.jsonExplanation so dry-run JSON is schema-compatible with
// check --format json.
type jsonDiagExp struct {
	Rule   string         `json:"rule"`
	Leaves []jsonDiagLeaf `json:"leaves"`
}

// jsonDiagLeaf is one leaf entry inside a jsonDiagExp.
type jsonDiagLeaf struct {
	Path   string `json:"path"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// diagsToJSONDiags converts lint diagnostics to the JSON shape used by
// dry-run output.
func diagsToJSONDiags(diags []lint.Diagnostic) []jsonDiag {
	jsonDiags := make([]jsonDiag, 0, len(diags))
	for _, d := range diags {
		var exp *jsonDiagExp
		if d.Explanation != nil {
			leaves := make([]jsonDiagLeaf, 0, len(d.Explanation.Leaves))
			for _, l := range d.Explanation.Leaves {
				leaves = append(leaves, jsonDiagLeaf{Path: l.Path, Value: l.Value, Source: l.Source})
			}
			exp = &jsonDiagExp{Rule: d.Explanation.Rule, Leaves: leaves}
		}
		jsonDiags = append(jsonDiags, jsonDiag{
			File:            d.File,
			Line:            d.Line,
			Column:          d.Column,
			Rule:            d.RuleID,
			Name:            d.RuleName,
			Severity:        string(d.Severity),
			Message:         d.Message,
			SourceLines:     d.SourceLines,
			SourceStartLine: d.SourceStartLine,
			Explanation:     exp,
			Deprecated:      d.Deprecated,
			ReplacedBy:      d.ReplacedBy,
		})
	}
	return jsonDiags
}

// writeDryRunJSON emits the per-file dry-run JSON payload on w as a
// JSON array of dryRunJSONFile records. Records are emitted for every
// file in WouldFixFiles plus any file that has remaining diagnostics
// not already covered. Callers route w to stderr to match the
// existing lint-output contract documented in
// docs/reference/cli.md. Returns a non-zero exit code on write error.
func writeDryRunJSON(w io.Writer, fixResult *fixpkg.Result) int {
	diagsByFile := make(map[string][]lint.Diagnostic)
	for _, d := range fixResult.Diagnostics {
		diagsByFile[d.File] = append(diagsByFile[d.File], d)
	}
	seen := make(map[string]struct{}, len(fixResult.WouldFixFiles))
	records := make([]dryRunJSONFile, 0, len(fixResult.WouldFixFiles))
	for _, wf := range fixResult.WouldFixFiles {
		seen[wf.Path] = struct{}{}
		rules := make([]string, 0, len(wf.Rules))
		for _, r := range wf.Rules {
			rules = append(rules, r.RuleID)
		}
		records = append(records, dryRunJSONFile{
			Path:        wf.Path,
			WouldFix:    wf.Count,
			Rules:       rules,
			Diagnostics: diagsToJSONDiags(diagsByFile[wf.Path]),
		})
	}
	// Include files with only unfixable diagnostics (not in WouldFixFiles).
	unfixable := make([]string, 0, len(diagsByFile))
	for path := range diagsByFile {
		if _, ok := seen[path]; !ok {
			unfixable = append(unfixable, path)
		}
	}
	sort.Strings(unfixable)
	for _, path := range unfixable {
		records = append(records, dryRunJSONFile{
			Path:        path,
			WouldFix:    0,
			Rules:       []string{},
			Diagnostics: diagsToJSONDiags(diagsByFile[path]),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(records); err != nil {
		// Best-effort routing of the encode error through the same
		// writer the payload was bound for, so callers that swap w
		// (tests with a fault-injecting writer; reportFixResultTo's
		// stderrW) keep all output confined to one destination.
		_, _ = fmt.Fprintf(w, "mdsmith: error writing dry-run output: %v\n", err)
		return 2
	}
	return 0
}
