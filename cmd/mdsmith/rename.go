package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/oscompat"
	"github.com/jeduden/mdsmith/internal/refactor"
)

// writeFileTempFn creates a named temp file; exposed as a variable so tests
// can inject failures without OS tricks.
var writeFileTempFn func(string, string) (*os.File, error) = os.CreateTemp

// writeFileTempFnMu guards reads and writes of writeFileTempFn so tests that
// swap it can coexist with parallel tests that call writeFilePreservingMode.
var writeFileTempFnMu sync.Mutex

// writeFileChmodFn sets permission bits on a file; exposed as a variable so
// tests can inject failures without OS tricks.
var writeFileChmodFn func(string, os.FileMode) error = oscompat.Chmod

// writeFileChmodFnMu guards reads and writes of writeFileChmodFn.
var writeFileChmodFnMu sync.Mutex

// writeFileWriteFn writes bytes to a file; exposed as a variable so tests
// can inject failures without OS tricks.
var writeFileWriteFn func(*os.File, []byte) (int, error) = (*os.File).Write

// writeFileWriteFnMu guards reads and writes of writeFileWriteFn.
var writeFileWriteFnMu sync.Mutex

// writeFileSyncFn syncs a file to disk; exposed as a variable so tests can
// inject failures without OS tricks.
var writeFileSyncFn func(*os.File) error = (*os.File).Sync

// writeFileSyncFnMu guards reads and writes of writeFileSyncFn.
var writeFileSyncFnMu sync.Mutex

// writeFileCloseFn closes a file; exposed as a variable so tests can inject
// failures without OS tricks.
var writeFileCloseFn func(*os.File) error = (*os.File).Close

// writeFileCloseFnMu guards reads and writes of writeFileCloseFn.
var writeFileCloseFnMu sync.Mutex

// writeFileRenameFn renames a staged temp file over its target; exposed as a
// variable so tests can inject failures without OS tricks.
var writeFileRenameFn func(string, string) error = os.Rename

// writeFileRenameFnMu guards reads and writes of writeFileRenameFn.
var writeFileRenameFnMu sync.Mutex

// renameOptions bundles the parsed CLI flags for `rename`.
type renameOptions struct {
	configPath   string
	format       string
	maxInputSize string
	as           string
	dryRun       bool
	walk         walkCLI
}

// renameSummary is one rewritten file's record for `--format json`.
type renameSummary struct {
	File  string `json:"file"`
	Edits int    `json:"edits"`
}

// cliRenameWorkspace backs the rename engine's Workspace seam with a
// transient index over the discovered files plus on-disk reads,
// mirroring how `deps` builds its graph. The key a file's edits group
// under is its workspace-relative path — the same string the CLI
// writes back to disk.
type cliRenameWorkspace struct {
	// IndexEdges (from refactor.NewLazyIndexEdges) builds the
	// transient index on its first edge query and reuses it after
	// that. Only the engine's edge and Files queries call it, so a
	// label rename — and Resolve or applyPlan — reads no file beyond
	// the ones it touches. A workspace built without an index answers
	// every edge query with nothing.
	refactor.IndexEdges
	relToAbs map[string]string
	rootDir  string
	maxBytes int64
}

func (w cliRenameWorkspace) Resolve(file string) (string, []byte, bool) {
	rel := index.NormalizePath(file)
	abs, ok := w.relToAbs[rel]
	if !ok {
		abs = filepath.Join(w.rootDir, filepath.FromSlash(rel))
	}
	src, err := bytelimit.ReadFileLimited(abs, w.maxBytes)
	if err != nil {
		return "", nil, false
	}
	return rel, src, true
}

// parseRenameFlags parses `mdsmith rename` flags and returns the
// options plus the remaining positional arguments.
func parseRenameFlags(args []string) (renameOptions, []string, error) {
	fs := flag.NewFlagSet("rename", flag.ContinueOnError)
	var (
		opts                        renameOptions
		noGitignore, followSymlinks bool
	)
	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&opts.format, "format", "f", "text", "Output format: text, json")
	fs.StringVar(&opts.as, "as", "",
		"What to rename: "+refactor.RenameKindList("%s")+" (auto-detected when omitted)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "Print the edits without writing them")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith rename [flags] <file> <old> <new>\n\n"+
			"Retitle a heading (rewriting every workspace anchor that targets it)\n"+
			"or rename a link-reference label (the def and every use in the file).\n"+
			"The kind is auto-detected; pass "+refactor.RenameKindList("--as %s")+" to force it.\n"+
			"To relocate a file, use mdsmith move.\n\n"+
			"  mdsmith rename docs/a.md \"Old Title\" \"New Title\"\n"+
			"  mdsmith rename docs/a.md --as label oldlabel newlabel\n\n"+
			"Exit codes: 0 rewritten, 1 no match or nothing to rename, 2 error or conflict\n\nFlags:\n")
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

// runRename implements the "rename" subcommand: retitle a heading or
// rename a link-reference label and rewrite every dependent edit in
// place. The kind is auto-detected unless --as forces it.
func runRename(args []string) int {
	opts, posArgs, err := parseRenameFlags(args)
	if err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: rename"); code >= 0 {
			return code
		}
	}
	kind, err := refactor.ParseRenameKind(opts.as)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: --as must be %s, got %q\n",
			refactor.RenameKindList("%s"), opts.as)
		return 2
	}
	if len(posArgs) != 3 {
		fmt.Fprint(os.Stderr, "mdsmith: rename requires <file> <old> <new>\n")
		return 2
	}
	target := normalizeWorkspacePath(posArgs[0])
	if !isWorkspaceRelativeTarget(target) {
		fmt.Fprintf(os.Stderr, "mdsmith: target %q must be workspace-relative\n", target)
		return 2
	}
	oldName, newName := posArgs[1], posArgs[2]

	ws, src, code := buildRenameWorkspace(opts, target)
	if code >= 0 {
		return code
	}

	plan, code := computeRenamePlan(ws, target, src, oldName, newName, kind)
	if code >= 0 {
		return code
	}
	return applyPlan(os.Stdout, ws, plan, opts.format, opts.dryRun)
}

// buildRenameWorkspace discovers the workspace, prepares the lazy
// transient index, and reads the target file's bytes. A non-negative return
// code means stop (1 = empty workspace, 2 = error); src is the target
// source on the success path.
func buildRenameWorkspace(opts renameOptions, target string) (cliRenameWorkspace, []byte, int) {
	ws, code := buildWorkspace(opts)
	if code >= 0 {
		return cliRenameWorkspace{}, nil, code
	}
	_, src, ok := ws.Resolve(target)
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: cannot read %q\n", target)
		return cliRenameWorkspace{}, nil, 2
	}
	return ws, src, -1
}

// buildWorkspace discovers the workspace and prepares the transient
// index shared by `rename` and `move` — built on first use, so a
// rename that never queries edges never indexes — without resolving
// any particular target file (each command resolves its own). A non-negative return
// code means stop (1 = empty workspace, 2 = error).
func buildWorkspace(opts renameOptions) (cliRenameWorkspace, int) {
	cfg, cfgPath, _, files, code := discoverFiles(opts.configPath, false, opts.walk)
	if code >= 0 {
		if code == 0 {
			fmt.Fprint(os.Stderr, "mdsmith: no Markdown files in workspace\n")
			return cliRenameWorkspace{}, 1
		}
		return cliRenameWorkspace{}, code
	}
	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return cliRenameWorkspace{}, 2
	}
	rootDir := rootDirFromConfig(cfgPath)
	relToAbs := make(map[string]string, len(files))
	rels := make([]string, 0, len(files))
	for _, srcPath := range files {
		rel := index.NormalizePath(workspaceRelativePath(srcPath, rootDir))
		relToAbs[rel] = srcPath
		rels = append(rels, rel)
	}
	edges := refactor.NewLazyIndexEdges(func() *index.Index {
		idx := index.New(rootDir)
		idx.BuildSerial(rels, func(rel string) ([]byte, error) {
			return bytelimit.ReadFileLimited(relToAbs[rel], maxBytes)
		})
		return idx
	})
	return cliRenameWorkspace{
		IndexEdges: edges,
		relToAbs:   relToAbs,
		rootDir:    rootDir,
		maxBytes:   maxBytes,
	}, -1
}

// computeRenamePlan runs the shared refactor.Rename dispatch — kind
// from --as, or "" to auto-detect from src — and maps its outcome to
// the CLI exit contract: 1 when an explicit kind finds nothing or the
// rename has no effect, 2 on a conflict, invalid input, an ambiguous
// or absent auto-detect, or a request that looks like a file move.
func computeRenamePlan(
	ws cliRenameWorkspace, target string, src []byte,
	oldName, newName string, kind refactor.RenameKind,
) (refactor.Plan, int) {
	plan, err := refactor.Rename(ws, target, src, kind, oldName, newName)
	if err != nil {
		return refactor.Plan{}, renameExitCode(err, target, oldName, newName)
	}
	return plan, -1
}

// renameExitCode prints the CLI message for a refactor.Rename error
// and returns its exit code. An absent auto-detect steers a
// path-shaped request to `mdsmith move`.
func renameExitCode(err error, target, oldName, newName string) int {
	var missing refactor.MissingSymbolError
	switch {
	case errors.Is(err, refactor.ErrAmbiguousRename):
		fmt.Fprintf(os.Stderr,
			"mdsmith: %q matches both %s in %s; pass %s\n",
			oldName, refactor.RenameSymbolList("and", true), target, refactor.RenameKindList("--as %s"))
		return 2
	case errors.Is(err, refactor.ErrNoRenameTarget):
		if looksLikePath(oldName) || looksLikePath(newName) {
			fmt.Fprintf(os.Stderr,
				"mdsmith: %q looks like a file path; to relocate a file use: mdsmith move %s %s\n",
				firstPathish(oldName, newName), oldName, newName)
			return 2
		}
		fmt.Fprintf(os.Stderr,
			"mdsmith: no %s %q in %s (to relocate a file, use mdsmith move)\n",
			refactor.RenameSymbolList("or", false), oldName, target)
		return 2
	case errors.Is(err, refactor.ErrNothingToRename):
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 1
	case errors.As(err, &missing):
		fmt.Fprintf(os.Stderr, "mdsmith: %v in %s\n", missing, target)
		return 1
	}
	fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
	return 2
}

// looksLikePath reports whether s reads as a file path rather than a
// heading or label name — it contains a slash or ends in a Markdown
// extension. rename's auto-detect uses it, only after finding no
// matching symbol, to steer a mistaken file rename to `mdsmith move`.
func looksLikePath(s string) bool {
	if strings.Contains(s, "/") {
		return true
	}
	return mdpath.HasMarkdownExt(filepath.Ext(s))
}

// firstPathish returns whichever of a, b looks like a path (a wins), for
// the move-intent hint message.
func firstPathish(a, b string) string {
	if looksLikePath(a) {
		return a
	}
	return b
}

// resolveWriteMode returns the permission bits to apply when creating a
// replacement file at path. For a symlink it follows to the live target; for a
// dangling symlink or any stat error it falls back to 0o644.
func resolveWriteMode(path string) os.FileMode {
	info, err := os.Lstat(path)
	if err != nil {
		return 0o644
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if tinfo, err := os.Stat(path); err == nil {
			return tinfo.Mode().Perm()
		}
		return 0o644
	}
	return info.Mode().Perm()
}

// writeFilePreservingMode overwrites path with data, keeping the file's
// existing permission bits.
//
// The write uses a temp-file-then-rename pattern: stageFile writes a
// temporary file in the same directory as path, and replaceWithStaged
// atomically renames it over path. On POSIX, os.Rename replaces the
// directory entry (symlink) itself rather than following the symlink to
// its target, so a workspace symlink is replaced with a regular file
// instead of overwriting the external target. This mirrors the
// atomicWriteFile pattern used by the fix command.
func writeFilePreservingMode(path string, data []byte) error {
	tmp, err := stageFile(path, data)
	if err != nil {
		return err
	}
	return replaceWithStaged(tmp, path)
}

// stageFile writes data to a new temp file beside path, carrying path's
// permission bits, synced and closed, and returns the temp file's name.
// path itself is not touched: the caller renames the temp over it
// (replaceWithStaged) or removes it. On error no temp file is left.
func stageFile(path string, data []byte) (string, error) {
	mode := resolveWriteMode(path)
	writeFileTempFnMu.Lock()
	createTemp := writeFileTempFn
	writeFileTempFnMu.Unlock()
	tmp, err := createTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	if err := fillTemp(tmp, mode, data); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// fillTemp sets tmp's permission bits to mode, writes data, syncs, and
// closes it. tmp is closed on every path.
func fillTemp(tmp *os.File, mode os.FileMode, data []byte) error {
	writeFileChmodFnMu.Lock()
	chmodFn := writeFileChmodFn
	writeFileChmodFnMu.Unlock()
	if err := chmodFn(tmp.Name(), mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting temp file mode: %w", err)
	}
	writeFileWriteFnMu.Lock()
	writeFn := writeFileWriteFn
	writeFileWriteFnMu.Unlock()
	if _, err := writeFn(tmp, data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	writeFileSyncFnMu.Lock()
	syncFn := writeFileSyncFn
	writeFileSyncFnMu.Unlock()
	if err := syncFn(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	writeFileCloseFnMu.Lock()
	closeFn := writeFileCloseFn
	writeFileCloseFnMu.Unlock()
	if err := closeFn(tmp); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	return nil
}

// replaceWithStaged renames the temp file tmp (from stageFile) over
// path, and removes tmp if the rename fails.
func replaceWithStaged(tmp, path string) error {
	writeFileRenameFnMu.Lock()
	rename := writeFileRenameFn
	writeFileRenameFnMu.Unlock()
	if err := rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("committing %s: %w", filepath.Base(path), err)
	}
	return nil
}
