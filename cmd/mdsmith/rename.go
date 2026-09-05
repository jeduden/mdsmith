package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/mdtext"
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
	idx      *index.Index
	relToAbs map[string]string
	rootDir  string
	maxBytes int64
}

// Trivial index pass-through; no dedicated test by design (covered
// by the heading-rename behavioral tests via the engine).
func (w cliRenameWorkspace) IncomingAnchorEdges(file, slug string) []index.Edge {
	return w.idx.IncomingEdges(file, slug)
}

// Trivial index pass-through; no dedicated test by design.
func (w cliRenameWorkspace) IncomingPathEdges(file string) []index.Edge {
	return w.idx.IncomingPathEdges(file)
}

// Trivial index pass-through; no dedicated test by design.
func (w cliRenameWorkspace) IncomingWikilinkEdges(stem string) []index.Edge {
	return w.idx.IncomingWikilinkEdges(stem)
}

// Trivial index pass-through; no dedicated test by design.
func (w cliRenameWorkspace) Files() []string { return w.idx.Files() }

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
		"What to rename: heading or label (auto-detected when omitted)")
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
			"The kind is auto-detected; pass --as heading or --as label to force it.\n"+
			"To relocate a file, use mdsmith move.\n\n"+
			"  mdsmith rename docs/a.md \"Old Title\" \"New Title\"\n"+
			"  mdsmith rename docs/a.md --as label oldlabel newlabel\n\n"+
			"Exit codes: 0 rewritten, 1 no match, 2 error or conflict\n\nFlags:\n")
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
	if opts.as != "" && opts.as != "heading" && opts.as != "label" {
		fmt.Fprintf(os.Stderr, "mdsmith: --as must be heading or label, got %q\n", opts.as)
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

	plan, code := computeRenamePlan(ws, target, src, oldName, newName, opts.as)
	if code >= 0 {
		return code
	}
	return applyPlan(os.Stdout, ws, plan, opts.format, opts.dryRun)
}

// buildRenameWorkspace discovers the workspace, builds the transient
// index, and reads the target file's bytes. A non-negative return
// code means stop (0 = empty workspace, 2 = error); src is the target
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

// buildWorkspace discovers the workspace and builds the transient index
// shared by `rename` and `move`, without resolving any particular
// target file (each command resolves its own). A non-negative return
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
	idx := index.New(rootDir)
	idx.BuildSerial(rels, func(rel string) ([]byte, error) {
		return bytelimit.ReadFileLimited(relToAbs[rel], maxBytes)
	})
	return cliRenameWorkspace{idx: idx, relToAbs: relToAbs, rootDir: rootDir, maxBytes: maxBytes}, -1
}

// computeRenamePlan resolves the rename mode — explicit --as, or
// auto-detected from src — and runs the engine, mapping a typed engine
// error to the CLI exit contract: 1 when an explicit mode finds
// nothing, 2 on a conflict, invalid input, an ambiguous auto-detect, or
// a request that looks like a file move.
func computeRenamePlan(
	ws cliRenameWorkspace, target string, src []byte,
	oldName, newName, as string,
) (refactor.Plan, int) {
	mode := as
	if mode == "" {
		m, code := detectRenameMode(target, src, oldName, newName)
		if code >= 0 {
			return refactor.Plan{}, code
		}
		mode = m
	}
	if mode == "heading" {
		return headingPlan(ws, target, src, oldName, newName)
	}
	return linkRefPlan(target, src, oldName, newName)
}

// detectRenameMode auto-detects whether oldName names a heading or a
// link-ref label in src. It returns the mode with code -1, or a
// non-negative exit code when the choice is ambiguous (both match),
// or absent (neither) — steering a path-shaped request to `mdsmith
// move`.
func detectRenameMode(target string, src []byte, oldName, newName string) (string, int) {
	_, isHeading := refactor.FindHeadingLine(src, oldName)
	isLabel := refactor.HasLinkRef(src, oldName)
	switch {
	case isHeading && isLabel:
		fmt.Fprintf(os.Stderr,
			"mdsmith: %q matches both a heading and a link-ref label in %s; pass --as heading or --as label\n",
			oldName, target)
		return "", 2
	case isHeading:
		return "heading", -1
	case isLabel:
		return "label", -1
	}
	if looksLikePath(oldName) || looksLikePath(newName) {
		fmt.Fprintf(os.Stderr,
			"mdsmith: %q looks like a file path; to relocate a file use: mdsmith move %s %s\n",
			firstPathish(oldName, newName), oldName, newName)
		return "", 2
	}
	fmt.Fprintf(os.Stderr,
		"mdsmith: no heading or link-ref label %q in %s (to relocate a file, use mdsmith move)\n",
		oldName, target)
	return "", 2
}

// headingPlan runs the heading rename, mapping a missing heading to
// exit 1 and an engine conflict to exit 2.
func headingPlan(ws cliRenameWorkspace, target string, src []byte, oldName, newName string) (refactor.Plan, int) {
	line, ok := refactor.FindHeadingLine(src, oldName)
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: no heading %q in %s\n", oldName, target)
		return refactor.Plan{}, 1
	}
	plan, err := refactor.Heading(ws, target, target, src, line, oldName, newName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return refactor.Plan{}, 2
	}
	if len(plan.Edits) == 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: nothing to rename for heading %q\n", oldName)
		return refactor.Plan{}, 1
	}
	return plan, -1
}

// linkRefPlan runs the link-ref rename, mapping a missing label to exit
// 1 and an engine conflict or invalid label to exit 2.
func linkRefPlan(target string, src []byte, oldName, newName string) (refactor.Plan, int) {
	plan, err := refactor.LinkRef(target, src, oldName, newName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return refactor.Plan{}, 2
	}
	if len(plan.Edits[target]) == 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: no link reference %q in %s\n", oldName, target)
		return refactor.Plan{}, 1
	}
	return plan, -1
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

// applyEdits splices every edit into src and returns the rewritten
// bytes. Each edit is single-line (heading text, label, or fragment).
// Edits on the same line are applied right-to-left so a left edit's
// byte offsets — computed against the original row — stay valid while
// the bytes to its right are rewritten. A trailing `\r` is preserved
// so CRLF files round-trip.
func applyEdits(src []byte, edits []refactor.Edit) ([]byte, error) {
	segs := splitKeepCR(src)
	byLine := map[int][]refactor.Edit{}
	for _, e := range edits {
		if e.Range.Start.Line != e.Range.End.Line {
			return nil, errors.New("multi-line edit is not supported")
		}
		byLine[e.Range.Start.Line] = append(byLine[e.Range.Start.Line], e)
	}
	for line, es := range byLine {
		if line < 0 || line >= len(segs) {
			return nil, fmt.Errorf("edit line %d out of range", line+1)
		}
		seg := segs[line]
		cr := len(seg) > 0 && seg[len(seg)-1] == '\r'
		row := seg
		if cr {
			row = seg[:len(seg)-1]
		}
		sortEditsByCharacterDesc(es)
		buf := append([]byte(nil), row...)
		for _, e := range es {
			s := mdtext.UTF16ToByteOffset(row, e.Range.Start.Character)
			en := mdtext.UTF16ToByteOffset(row, e.Range.End.Character)
			if s < 0 || en < 0 || s > len(buf) || en > len(buf) || s > en {
				return nil, fmt.Errorf("edit offset [%d,%d) out of range on line %d", s, en, line+1)
			}
			next := make([]byte, 0, len(buf)-(en-s)+len(e.NewText))
			next = append(next, buf[:s]...)
			next = append(next, e.NewText...)
			next = append(next, buf[en:]...)
			buf = next
		}
		if cr {
			buf = append(buf, '\r')
		}
		segs[line] = buf
	}
	return joinLF(segs), nil
}

// sortEditsByCharacterDesc orders es by descending Start.Character in
// place (rightmost edit first), so applyEdits can splice each edit
// into the line without its offset shifting from an earlier splice.
// slices.SortStableFunc compares the concrete refactor.Edit values
// directly, unlike sort.SliceStable, which drives reflect.Swapper
// under the hood — see docs/development/high-performance-go.md's
// "reflect in hot paths" anti-pattern. Stability preserves the
// original order among edits reported at the same offset.
func sortEditsByCharacterDesc(es []refactor.Edit) {
	slices.SortStableFunc(es, func(a, b refactor.Edit) int {
		return cmp.Compare(b.Range.Start.Character, a.Range.Start.Character)
	})
}

// splitKeepCR splits src on `\n`, keeping any trailing `\r` on each
// segment so CRLF endings survive a round-trip.
func splitKeepCR(src []byte) [][]byte {
	var segs [][]byte
	start := 0
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			segs = append(segs, src[start:i])
			start = i + 1
		}
	}
	segs = append(segs, src[start:])
	return segs
}

// joinLF rejoins segments with `\n`, the inverse of splitKeepCR.
func joinLF(segs [][]byte) []byte {
	var out []byte
	for i, s := range segs {
		if i > 0 {
			out = append(out, '\n')
		}
		out = append(out, s...)
	}
	return out
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
// The write uses a temp-file-then-rename pattern: a temporary file is created
// in the same directory as path, written, then atomically renamed over path.
// On POSIX, os.Rename replaces the directory entry (symlink) itself rather
// than following the symlink to its target, so a workspace symlink is replaced
// with a regular file instead of overwriting the external target. This mirrors
// the atomicWriteFile pattern used by the fix command.
func writeFilePreservingMode(path string, data []byte) error {
	mode := resolveWriteMode(path)
	dir := filepath.Dir(path)
	writeFileTempFnMu.Lock()
	createTemp := writeFileTempFn
	writeFileTempFnMu.Unlock()
	tmp, err := createTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // best-effort cleanup; harmless once rename succeeds
	writeFileChmodFnMu.Lock()
	chmodFn := writeFileChmodFn
	writeFileChmodFnMu.Unlock()
	if err := chmodFn(tmpName, mode); err != nil {
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
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("committing %s: %w", filepath.Base(path), err)
	}
	return nil
}
