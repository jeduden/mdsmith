package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

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
			"Move a Markdown file and rewrite every reference in one step: incoming\n"+
			"links and ref-def destinations across the workspace, and the moved file's\n"+
			"own outbound relative links. A tracked file is staged with git mv.\n\n"+
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

// applyPlan is the shared apply layer for refactor plans. It writes
// every keyed file's edits, then runs any FileOp (a git mv or plain
// rename) — text edits before the move, so the relocated file carries
// its rewritten body. With dryRun set it changes nothing and only
// reports what it would do. Returns 0 on success, 2 on a write or move
// failure.
func applyPlan(w io.Writer, ws cliRenameWorkspace, plan refactor.Plan, format string, dryRun bool) int {
	rels := make([]string, 0, len(plan.Edits))
	for rel, edits := range plan.Edits {
		if len(edits) == 0 {
			continue
		}
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	// Pre-flight the file move before writing any reference edits.
	// Execute refuses an existing destination (git mv does; the
	// plain-rename path mirrors it with an Lstat guard), but by then
	// every reference edit is already on disk, pointing at a file the
	// move never created — corrupting the workspace with no rollback.
	// Checking here keeps the operation all-or-nothing for the common
	// collision the planner's read-based check cannot see (a destination
	// over the max-input-size limit, which ws.Resolve reports as absent).
	if !dryRun && plan.FileOp != nil {
		dst := filepath.Join(ws.rootDir, filepath.FromSlash(plan.FileOp.To))
		if _, err := os.Lstat(dst); err == nil {
			fmt.Fprintf(os.Stderr,
				"mdsmith: destination already exists: %s\n", plan.FileOp.To)
			return 2
		}
	}

	summaries := make([]renameSummary, 0, len(rels))
	for _, rel := range rels {
		edits := plan.Edits[rel]
		if !dryRun {
			if code := applyEditsToFile(ws, rel, edits); code != 0 {
				return code
			}
		}
		summaries = append(summaries, renameSummary{File: rel, Edits: len(edits)})
	}
	if !dryRun && plan.FileOp != nil {
		if err := plan.FileOp.Execute(ws.rootDir); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return 2
		}
	}
	return emitPlanReport(w, summaries, plan.FileOp, format, dryRun)
}

// applyEditsToFile splices one file's edits and writes the result back,
// preserving the file's mode. Returns 0 on success, 2 on a read, apply,
// or write failure.
func applyEditsToFile(ws cliRenameWorkspace, rel string, edits []refactor.Edit) int {
	_, src, ok := ws.Resolve(rel)
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: cannot read %q to apply edits\n", rel)
		return 2
	}
	out, err := applyEdits(src, edits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %s: %v\n", rel, err)
		return 2
	}
	abs, ok := ws.relToAbs[rel]
	if !ok {
		abs = filepath.Join(ws.rootDir, filepath.FromSlash(rel))
	}
	if err := writeFilePreservingMode(abs, out); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing %s: %v\n", rel, err)
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
