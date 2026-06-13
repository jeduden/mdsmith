package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/oscompat"
	"github.com/jeduden/mdsmith/internal/rename"
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
	heading      bool
	linkRef      bool
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
	fs.BoolVar(&opts.heading, "heading", false, "Rename a heading and every workspace anchor that targets it")
	fs.BoolVar(&opts.linkRef, "link-ref", false, "Rename a link-reference label: the def and every use in the file")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith rename [flags] <file> <old> <new>\n\n"+
			"Rename a heading or a link-reference label, rewriting every\n"+
			"dependent edit across the workspace in place. Exactly one of\n"+
			"--heading or --link-ref is required.\n\n"+
			"  mdsmith rename docs/a.md --heading \"Old Title\" \"New Title\"\n"+
			"  mdsmith rename docs/a.md --link-ref oldlabel newlabel\n\n"+
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

// runRename implements the "rename" subcommand: rename a heading or a
// link-reference label and rewrite every dependent edit in place.
func runRename(args []string) int {
	opts, posArgs, err := parseRenameFlags(args)
	if err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: rename"); code >= 0 {
			return code
		}
	}
	if opts.heading == opts.linkRef {
		fmt.Fprint(os.Stderr, "mdsmith: rename requires exactly one of --heading or --link-ref\n")
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

	changes, code := computeRenameChanges(ws, target, src, oldName, newName, opts.heading)
	if code >= 0 {
		return code
	}
	return applyAndReport(os.Stdout, ws, changes, opts.format)
}

// buildRenameWorkspace discovers the workspace, builds the transient
// index, and reads the target file's bytes. A non-negative return
// code means stop (0 = empty workspace, 2 = error); src is the target
// source on the success path.
func buildRenameWorkspace(opts renameOptions, target string) (cliRenameWorkspace, []byte, int) {
	cfg, cfgPath, _, files, code := discoverFiles(opts.configPath, false, opts.walk)
	if code >= 0 {
		if code == 0 {
			fmt.Fprint(os.Stderr, "mdsmith: no Markdown files in workspace\n")
			return cliRenameWorkspace{}, nil, 1
		}
		return cliRenameWorkspace{}, nil, code
	}
	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return cliRenameWorkspace{}, nil, 2
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
	ws := cliRenameWorkspace{idx: idx, relToAbs: relToAbs, rootDir: rootDir, maxBytes: maxBytes}
	_, src, ok := ws.Resolve(target)
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: cannot read %q\n", target)
		return cliRenameWorkspace{}, nil, 2
	}
	return ws, src, -1
}

// computeRenameChanges runs the rename engine for the requested mode
// and maps a typed engine error to the CLI exit contract: 1 when
// nothing matched, 2 on a conflict or invalid input.
func computeRenameChanges(
	ws cliRenameWorkspace, target string, src []byte,
	oldName, newName string, isHeading bool,
) (map[string][]rename.Edit, int) {
	if isHeading {
		line, ok := rename.FindHeadingLine(src, oldName)
		if !ok {
			fmt.Fprintf(os.Stderr, "mdsmith: no heading %q in %s\n", oldName, target)
			return nil, 1
		}
		changes, err := rename.Heading(ws, target, target, src, line, oldName, newName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return nil, 2
		}
		if len(changes) == 0 {
			fmt.Fprintf(os.Stderr, "mdsmith: nothing to rename for heading %q\n", oldName)
			return nil, 1
		}
		return changes, -1
	}
	edits, err := rename.LinkRef(src, rename.NormalizeLabel(oldName), newName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, 2
	}
	if len(edits) == 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: no link reference %q in %s\n", oldName, target)
		return nil, 1
	}
	return map[string][]rename.Edit{target: edits}, -1
}

// applyAndReport writes every change to disk and prints the per-file
// summary. Returns 0 on success, 2 on a write or render failure.
func applyAndReport(
	w io.Writer, ws cliRenameWorkspace,
	changes map[string][]rename.Edit, format string,
) int {
	summaries := make([]renameSummary, 0, len(changes))
	for rel, edits := range changes {
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
		summaries = append(summaries, renameSummary{File: rel, Edits: len(edits)})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].File < summaries[j].File })
	return emitRenameSummary(w, summaries, format)
}

// emitRenameSummary renders the rewritten-file list. Exit code: 0 on
// success, 2 on unknown format or write error.
func emitRenameSummary(w io.Writer, summaries []renameSummary, format string) int {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summaries); err != nil {
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
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: unknown --format %q (want text or json)\n", format)
		return 2
	}
	return 0
}

// applyEdits splices every edit into src and returns the rewritten
// bytes. Each edit is single-line (heading text, label, or fragment).
// Edits on the same line are applied right-to-left so a left edit's
// byte offsets — computed against the original row — stay valid while
// the bytes to its right are rewritten. A trailing `\r` is preserved
// so CRLF files round-trip.
func applyEdits(src []byte, edits []rename.Edit) ([]byte, error) {
	segs := splitKeepCR(src)
	byLine := map[int][]rename.Edit{}
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
		sort.SliceStable(es, func(i, j int) bool {
			return es[i].Range.Start.Character > es[j].Range.Start.Character
		})
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
