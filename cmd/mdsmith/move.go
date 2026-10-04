package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/refactor"
)

// moveOptions bundles the parsed CLI flags for `move`.
type moveOptions struct {
	configPath   string
	format       string
	maxInputSize string
	dryRun       bool
	walk         walkCLI
}

// parseMoveFlags parses `mdsmith move` flags and returns the options
// plus the remaining positional arguments.
func parseMoveFlags(args []string) (moveOptions, []string, error) {
	fs := flag.NewFlagSet("move", flag.ContinueOnError)
	var (
		opts                        moveOptions
		noGitignore, followSymlinks bool
	)
	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&opts.format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "Print the edits and planned move; change nothing")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith move [flags] <src> <dst>\n\n"+
			"Move a workspace file and rewrite every reference in one step: incoming\n"+
			"links and ref-def destinations across the workspace, a moved Markdown\n"+
			"file's own outbound relative links, and wikilinks when the basename\n"+
			"changes ([[stem]] for Markdown, typed [[name.ext]] for other files).\n"+
			"A tracked file is staged with git mv.\n\n"+
			"  mdsmith move docs/old.md docs/new.md\n"+
			"  mdsmith move guide.md reference/guide.md --dry-run\n\n"+
			"Exit codes: 0 moved, 1 source not found, 2 error or conflict\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	opts.walk = walkCLI{
		noGitignore:    noGitignore,
		followSymlinks: followSymlinksOverride(fs, followSymlinks),
	}
	return opts, fs.Args(), nil
}

// runMove implements the "move" subcommand: relocate a workspace file
// and rewrite every reference, then stage the rename (git mv when
// tracked, plain rename otherwise).
func runMove(args []string) int {
	opts, posArgs, err := parseMoveFlags(args)
	if err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: move"); code >= 0 {
			return code
		}
	}
	if len(posArgs) != 2 {
		fmt.Fprint(os.Stderr, "mdsmith: move requires <src> <dst>\n")
		return 2
	}
	src := normalizeWorkspacePath(posArgs[0])
	dst := normalizeWorkspacePath(posArgs[1])
	for _, p := range []string{src, dst} {
		if !isWorkspaceRelativeTarget(p) {
			fmt.Fprintf(os.Stderr, "mdsmith: path %q must be workspace-relative\n", p)
			return 2
		}
	}

	ws, code := buildWorkspace(renameOptions{
		configPath:   opts.configPath,
		maxInputSize: opts.maxInputSize,
		walk:         opts.walk,
	})
	if code >= 0 {
		return code
	}

	plan, err := refactor.Move(ws, src, dst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		var snf refactor.SourceNotFoundError
		if errors.As(err, &snf) {
			return 1
		}
		return 2
	}
	return applyPlan(os.Stdout, ws, plan, opts.format, opts.dryRun)
}

// planReport is the `--format json` shape shared by move (and, once it
// adopts applyPlan, rename): the per-file edit counts plus an optional
// file move and a dry-run marker.
type planReport struct {
	Files  []renameSummary `json:"files"`
	Move   *fileMoveReport `json:"move,omitempty"`
	DryRun bool            `json:"dryRun,omitempty"`
}

// fileMoveReport is the JSON view of a planned or performed file move.
type fileMoveReport struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// applyPlan is the shared apply layer for refactor plans, and it is
// all-or-nothing (see planapply.go). It refuses an existing
// destination, splices every file's edits in memory, and aborts with
// nothing written if any file fails. With dryRun set it stops there
// and reports what it would do, so a dry run surfaces the same errors.
// Otherwise commitPlan writes the files and runs any FileOp (a git mv
// or plain rename) last, so the relocated file carries its rewritten
// body; a failure there rolls back and names any file left rewritten.
// Returns 0 on success, 2 on any failure.
func applyPlan(w io.Writer, ws cliRenameWorkspace, plan refactor.Plan, format string, dryRun bool) int {
	if code := preflightDestination(ws.rootDir, plan.FileOp); code != 0 {
		return code
	}
	writes, code := computePlanWrites(ws, plan.Edits)
	if code != 0 {
		return code
	}
	if !dryRun {
		if f := commitPlan(ws.rootDir, writes, plan.FileOp); f != nil {
			fmt.Fprint(os.Stderr, f.message())
			return 2
		}
	}
	summaries := make([]renameSummary, 0, len(writes))
	for _, pw := range writes {
		summaries = append(summaries, renameSummary{File: pw.rel, Edits: pw.edits})
	}
	return emitPlanReport(w, summaries, plan.FileOp, format, dryRun)
}

// preflightDestination refuses a file move whose destination already
// exists, before anything is written. Execute refuses one too (git mv
// does; the plain-rename path mirrors it with an Lstat guard), but only
// after the reference edits are written, so this check keeps the
// common collision from costing a rollback. It also refuses a
// case-only rename the planner plans as the source itself on a
// case-insensitive file system (see refactor.MoveAll): Lstat finds the
// source under the new spelling, and Execute would refuse it too.
// Returns 0 when op is nil or its destination is free, 2 otherwise.
func preflightDestination(rootDir string, op *refactor.FileOp) int {
	if op == nil {
		return 0
	}
	dst := filepath.Join(rootDir, filepath.FromSlash(op.To))
	if _, err := os.Lstat(dst); err == nil {
		fmt.Fprintf(os.Stderr, "mdsmith: destination already exists: %s\n", op.To)
		return 2
	}
	return 0
}

// emitPlanReport renders the rewritten-file list plus any file move.
// Text prints one `file: N edit(s)` line per file, then a move line
// ("moved" applied, "would move" for a dry run). JSON emits a
// planReport. Exit code: 0 on success, 2 on unknown format or write
// error.
func emitPlanReport(w io.Writer, summaries []renameSummary, op *refactor.FileOp, format string, dryRun bool) int {
	switch format {
	case "json":
		rep := planReport{Files: summaries, DryRun: dryRun}
		if op != nil {
			rep.Move = &fileMoveReport{From: op.From, To: op.To}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: writing json: %v\n", err)
			return 2
		}
	case "text", "":
		for _, s := range summaries {
			if _, err := fmt.Fprintf(w, "%s: %d edit(s)\n", s.File, s.Edits); err != nil {
				fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
				return 2
			}
		}
		if op != nil {
			verb := "moved"
			if dryRun {
				verb = "would move"
			}
			if _, err := fmt.Fprintf(w, "%s %s -> %s\n", verb, op.From, op.To); err != nil {
				fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
				return 2
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: unknown --format %q (want text or json)\n", format)
		return 2
	}
	return 0
}
