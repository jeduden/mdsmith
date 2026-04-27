package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

const mergeDriverUsage = `Usage: mdsmith merge-driver <subcommand> [args]

Subcommands:
  run <base> <ours> <theirs> <pathname>
        Run as a git custom merge driver. Strips conflict
        markers inside regenerable sections (catalog, include),
        runs mdsmith fix to regenerate them, and exits non-zero
        if unresolved conflict markers remain.

  install [files...]
        Register the merge driver in git config and ensure
        .gitattributes assigns it to the listed files.
        Default files: PLAN.md README.md

Git config (set by install):
  merge.mdsmith.driver = '/absolute/path/to/mdsmith' merge-driver run %O %A %B %P

  The path is the absolute location of the mdsmith binary at install time,
  shell-quoted so paths with spaces are handled correctly.
`

// runMergeDriver dispatches the merge-driver subcommand.
func runMergeDriver(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, mergeDriverUsage)
		return 0
	}

	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(os.Stderr, mergeDriverUsage)
		return 0
	case "run":
		return runMergeDriverRun(args[1:])
	case "install":
		return runMergeDriverInstall(args[1:])
	default:
		fmt.Fprintf(os.Stderr,
			"mdsmith: merge-driver: unknown subcommand %q\n\n%s",
			args[0], mergeDriverUsage)
		return 2
	}
}

// mergeAndClean performs the 3-way merge and strips conflict markers.
// Returns the cleaned content and an exit code (0 on success).
func mergeAndClean(base, ours, theirs string, maxBytes int64) ([]byte, int) {
	// Step 1: standard 3-way merge into ours.
	mergeCmd := exec.Command("git", "merge-file", ours, base, theirs)
	mergeCmd.Stderr = os.Stderr
	mergeErr := mergeCmd.Run()

	// git merge-file exits 1 for conflicts, 2+ for fatal errors.
	// Non-ExitError (e.g. git not found) is also fatal.
	if mergeErr != nil {
		if exitErr, ok := mergeErr.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			fmt.Fprintf(os.Stderr, "mdsmith: git merge-file failed: %v\n", mergeErr)
			return nil, 2
		}
	}

	// Step 2: strip conflict markers inside regenerable sections.
	content, err := lint.ReadFileLimited(ours, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading merge result: %v\n", err)
		return nil, 2
	}

	cleaned := stripSectionConflicts(content)
	if err := os.WriteFile(ours, cleaned, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing cleaned merge: %v\n", err)
		return nil, 2
	}
	return cleaned, 0
}

// runMergeDriverRun implements the git merge driver protocol.
// Arguments: <base> <ours> <theirs> <pathname>
//
// git calls this with %O %A %B %P where:
//   - %O = ancestor (temp file)
//   - %A = ours (temp file, write result here)
//   - %B = theirs (temp file)
//   - %P = pathname in the working tree
func runMergeDriverRun(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, mergeDriverUsage)
		return 0
	}

	if len(args) < 4 {
		fmt.Fprintf(os.Stderr,
			"mdsmith: merge-driver run requires 4 arguments: "+
				"base ours theirs pathname\n")
		return 2
	}

	base, ours, theirs, pathname := args[0], args[1], args[2], args[3]

	// Resolve the effective max-input-size from config so the merge
	// driver honors the same limit as check/fix.
	cfg, _, err := loadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: loading config: %v\n", err)
		return 2
	}
	maxBytes, err := resolveMaxInputBytes(cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	cleaned, rc := mergeAndClean(base, ours, theirs, maxBytes)
	if rc != 0 {
		return rc
	}

	// Step 3: run mdsmith fix at the real path and write result
	// back to ours.
	fixed, rc := fixAtRealPath(cleaned, ours, pathname, maxBytes)
	if rc != 0 {
		return rc
	}

	// Step 4: check for remaining conflict markers.
	if hasConflictMarkers(fixed) {
		fmt.Fprintf(os.Stderr,
			"mdsmith: unresolved conflict markers remain in %s\n",
			pathname)
		return 1
	}

	return 0
}

// fixAtRealPath writes cleaned content to pathname, runs mdsmith
// fix, copies the result to ours, and restores pathname.
func fixAtRealPath(cleaned []byte, ours, pathname string, maxBytes int64) ([]byte, int) {
	// Capture the original file mode so we can preserve permissions.
	fileMode := os.FileMode(0644)
	if info, err := os.Stat(pathname); err == nil {
		fileMode = info.Mode()
	}

	backup, backupErr := lint.ReadFileLimited(pathname, maxBytes)
	if backupErr != nil && !os.IsNotExist(backupErr) {
		fmt.Fprintf(os.Stderr, "mdsmith: reading %s for backup: %v\n", pathname, backupErr)
		return nil, 2
	}
	if err := os.WriteFile(pathname, cleaned, fileMode); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing to %s: %v\n", pathname, err)
		return nil, 2
	}

	fixErr := fixFileInPlace(pathname, maxBytes)

	// Restore the original working tree file before checking
	// fixErr, so the working tree is always left clean.
	fixed, err := lint.ReadFileLimited(pathname, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading fixed file: %v\n", err)
		return nil, 2
	}

	var restoreErr error
	if backupErr == nil {
		restoreErr = os.WriteFile(pathname, backup, fileMode)
	} else if os.IsNotExist(backupErr) {
		restoreErr = os.Remove(pathname)
	}
	if restoreErr != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: restoring %s: %v\n", pathname, restoreErr)
		return fixed, 2
	}

	// Check fixErr before writing to ours so broken content is
	// not used as the merge result.
	if fixErr != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: fix failed: %v\n", fixErr)
		return fixed, 2
	}

	if err := os.WriteFile(ours, fixed, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing merge output: %v\n", err)
		return nil, 2
	}

	return fixed, 0
}

// fixFileInPlace runs the mdsmith fix pipeline on a single file.
func fixFileInPlace(path string, maxBytes int64) error {
	cfg, _, err := loadConfig("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           &vlog.Logger{},
		MaxInputBytes:    maxBytes,
	}

	result := fixer.Fix([]string{path})
	if len(result.Errors) > 0 {
		return result.Errors[0]
	}
	return nil
}

// regenDirectiveNames returns the directive names whose content
// is regenerated by mdsmith fix. Names are discovered from
// registered rules that implement gensection.Directive.
func regenDirectiveNames() []string {
	var names []string
	for _, r := range rule.All() {
		if d, ok := r.(gensection.Directive); ok {
			names = append(names, d.Name())
		}
	}
	return names
}

// stripSectionConflicts removes git conflict markers from lines
// that fall inside regenerable sections. Section names are
// discovered dynamically from registered gensection.Directive rules.
// Conflict markers outside these sections are left unchanged.
//
// Both standard (<<<<<<<, =======, >>>>>>>) and diff3
// (<<<<<<<, |||||||, =======, >>>>>>>) conflict styles are supported.
//
// The ======= separator is only stripped when it appears between
// <<<<<<< and >>>>>>> to avoid false positives with Markdown
// setext heading underlines.
func stripSectionConflicts(content []byte) []byte {
	names := regenDirectiveNames()
	lines := bytes.Split(content, []byte("\n"))
	var out [][]byte
	inSection := false
	inConflict := false

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)

		if matchesAnyStart(trimmed, names) {
			inSection = true
		}

		if inSection {
			if isConflictOpen(trimmed) {
				inConflict = true
				continue
			}
			if inConflict && isConflictClose(trimmed) {
				inConflict = false
				continue
			}
			if inConflict && isConflictBase(trimmed) {
				continue
			}
			if inConflict && isConflictSeparator(trimmed) {
				continue
			}
		}

		out = append(out, line)

		if matchesAnyEnd(trimmed, names) {
			inSection = false
			inConflict = false
		}
	}

	return bytes.Join(out, []byte("\n"))
}

func matchesAnyStart(line []byte, names []string) bool {
	for _, name := range names {
		if gensection.IsRawStartMarker(line, name) {
			return true
		}
	}
	return false
}

func matchesAnyEnd(line []byte, names []string) bool {
	for _, name := range names {
		if gensection.IsRawEndMarker(line, name) {
			return true
		}
	}
	return false
}

// isConflictOpen returns true if the line opens a git conflict
// block (starts with <<<<<<<).
func isConflictOpen(line []byte) bool {
	return bytes.HasPrefix(line, []byte("<<<<<<<"))
}

// isConflictBase returns true if the line opens the base
// (ancestor) section in a diff3-style conflict block (starts
// with |||||||). This marker only appears when git is
// configured with merge.conflictstyle = diff3 or zdiff3.
func isConflictBase(line []byte) bool {
	return bytes.HasPrefix(line, []byte("|||||||"))
}

// isConflictSeparator returns true if the line is a git conflict
// separator (starts with =======). This is context-dependent:
// the same pattern is valid as a Markdown setext heading
// underline, so callers must check conflict state.
func isConflictSeparator(line []byte) bool {
	return bytes.HasPrefix(line, []byte("======="))
}

// isConflictClose returns true if the line closes a git conflict
// block (starts with >>>>>>>).
func isConflictClose(line []byte) bool {
	return bytes.HasPrefix(line, []byte(">>>>>>>"))
}

// hasConflictMarkers returns true if the content contains any
// git conflict markers. Only checks for unambiguous markers
// (<<<<<<< and >>>>>>>) to avoid setext heading false positives.
func hasConflictMarkers(content []byte) bool {
	for _, line := range bytes.Split(content, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if isConflictOpen(trimmed) || isConflictClose(trimmed) {
			return true
		}
	}
	return false
}

// defaultMergeDriverFiles are the files assigned to the merge
// driver when install is run without explicit file arguments.
var defaultMergeDriverFiles = []string{"PLAN.md", "README.md"}

// runMergeDriverInstall registers the mdsmith merge driver in
// the local git config and ensures .gitattributes assigns it.
func runMergeDriverInstall(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, mergeDriverUsage)
		return 0
	}

	// Verify we're in a git repo.
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: not in a git repository\n")
		return 2
	}
	repoRoot := strings.TrimSpace(string(out))

	if err := registerMergeDriver(); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	// Determine file list: use args if given, else defaults.
	files := defaultMergeDriverFiles
	if len(args) > 0 {
		files = args
	}

	attrPath := filepath.Join(repoRoot, ".gitattributes")
	if err := ensureGitattributes(attrPath, files); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: updating .gitattributes: %v\n", err)
		return 2
	}

	if err := ensurePreMergeCommitHook(repoRoot, files); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: installing pre-merge-commit hook: %v\n", err)
		return 2
	}

	hookPath := filepath.Join(resolveHooksDir(repoRoot), "pre-merge-commit")
	fmt.Fprintf(os.Stderr, "mdsmith: merge driver 'mdsmith' installed\n")
	fmt.Fprintf(os.Stderr, "  git config: merge.mdsmith.driver\n")
	fmt.Fprintf(os.Stderr, "  .gitattributes: %s\n", attrPath)
	fmt.Fprintf(os.Stderr, "  pre-merge-commit hook: %s\n", hookPath)
	return 0
}

// preMergeCommitHookMarker identifies the hook as managed by
// mdsmith so re-running install can safely replace it without
// stomping on a user-authored hook of the same name.
const preMergeCommitHookMarker = "# mdsmith merge-driver pre-merge-commit hook"

// resolveHooksDir returns the directory where git hooks should be
// installed. It respects core.hooksPath if configured so that
// installations work correctly in repos that redirect hooks to a
// custom path (e.g. via git config or a repo management tool).
// Falls back to .git/hooks when git cannot be queried.
func resolveHooksDir(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-path", "hooks")
	if out, err := cmd.Output(); err == nil {
		p := strings.TrimSpace(string(out))
		if !filepath.IsAbs(p) {
			p = filepath.Join(repoRoot, p)
		}
		return filepath.Clean(p)
	}
	return filepath.Join(repoRoot, ".git", "hooks")
}

// ensurePreMergeCommitHook writes the pre-merge-commit hook so
// that after git resolves all per-file merges (including any
// driver-resolved sections) and before the merge commit is
// created, mdsmith fix runs once on the registered files.
//
// The per-file merge driver cannot do this on its own: when it
// runs on PLAN.md, sibling plan/*.md source files may still hold
// "ours" content because git has not merged them yet, so the
// regenerated catalog reflects a stale view of its sources. The
// pre-merge-commit hook re-fixes the same files once every path
// has reached its final merged state.
func ensurePreMergeCommitHook(repoRoot string, files []string) error {
	exe, err := resolveInstalledBinary()
	if err != nil {
		return fmt.Errorf("cannot locate mdsmith binary: %w", err)
	}

	hooksDir := resolveHooksDir(repoRoot)
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")

	// Refuse to clobber a hook the user wrote themselves; replace
	// only hooks that carry our marker. A non-ENOENT read error is
	// treated as a safety failure to avoid silently overwriting an
	// unreadable hook.
	existing, readErr := os.ReadFile(hookPath)
	switch {
	case readErr == nil:
		if !strings.Contains(string(existing), preMergeCommitHookMarker) {
			return fmt.Errorf(
				"%s already exists and is not managed by mdsmith; "+
					"remove or merge it manually",
				hookPath)
		}
	case os.IsNotExist(readErr):
		// Hook doesn't exist; safe to create.
	default:
		return fmt.Errorf("reading existing hook %s: %w", hookPath, readErr)
	}

	// Build per-file fix commands as separate lines so that "set -e"
	// aborts the hook if mdsmith fix or git add fails. Files that no
	// longer exist (e.g. renamed in this branch) are skipped.
	var fixCmds strings.Builder
	for _, f := range files {
		fmt.Fprintf(&fixCmds,
			"if [ -e %s ]; then\n  %s fix -- %s\n  git add -- %s\nfi\n",
			shellQuote(f), shellQuote(exe), shellQuote(f), shellQuote(f))
	}

	content := "#!/bin/sh\n" +
		preMergeCommitHookMarker + "\n" +
		"# Re-runs mdsmith fix once git has resolved every per-file\n" +
		"# merge, so generated sections reflect the final merged\n" +
		"# state of every source file. Re-install with:\n" +
		"#   mdsmith merge-driver install\n" +
		"set -e\n" +
		fixCmds.String()

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", hooksDir, err)
	}
	if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("writing %s: %w", hookPath, err)
	}
	return nil
}

// registerMergeDriver writes the merge.mdsmith.* keys to local
// git config. It uses the absolute path of the current executable
// so the driver works regardless of whether the install directory
// is in PATH.
func registerMergeDriver() error {
	exe, err := resolveInstalledBinary()
	if err != nil {
		return fmt.Errorf("cannot locate mdsmith binary: %w", err)
	}
	driver := shellQuote(exe) + " merge-driver run %O %A %B %P"
	cmds := [][]string{
		{"git", "config", "merge.mdsmith.name",
			"mdsmith section-aware Markdown merge"},
		{"git", "config", "merge.mdsmith.driver", driver},
	}
	for _, c := range cmds {
		if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
			return fmt.Errorf("git config failed: %w", err)
		}
	}
	return nil
}

// executableFunc is the function used to locate the current binary.
// Overridden in tests to exercise the non-temporary-exe branch.
var executableFunc = os.Executable

// resolveInstalledBinary returns the absolute path to the mdsmith
// binary to use as the git merge driver. It prefers the current
// executable when it lives outside the OS temp directory (i.e. it
// was installed via "go install" or a release download). When the
// current executable is a transient "go run" binary it falls back
// to searching PATH and then $GOPATH/bin.
func resolveInstalledBinary() (string, error) {
	if exe, err := executableFunc(); err == nil {
		if !isTemporaryBinary(exe) {
			return filepath.Clean(exe), nil
		}
	}
	// Transient go-run binary — try PATH first, then $GOPATH/bin.
	if p, err := exec.LookPath("mdsmith"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs, nil
		}
	}
	gopath, err := goEnvPath()
	if err == nil {
		// GOPATH may contain multiple entries separated by os.PathListSeparator.
		// Check each entry's bin/mdsmith.
		for _, entry := range filepath.SplitList(gopath) {
			if entry == "" {
				continue
			}
			candidate := filepath.Join(entry, "bin", "mdsmith")
			if p, err := exec.LookPath(candidate); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf(
		"mdsmith not found in PATH or $GOPATH/bin; " +
			"run \"go install ./cmd/mdsmith\" first",
	)
}

// isTemporaryBinary reports whether path looks like a transient binary
// created by "go run" or "go test" (i.e. built into a go-build/go-run
// subdirectory under the OS temp directory). Binaries merely downloaded
// to TempDir by a user or CI script are not considered transient.
func isTemporaryBinary(path string) bool {
	tmp := filepath.Clean(os.TempDir())
	path = filepath.Clean(path)
	rel, err := filepath.Rel(tmp, path)
	if err != nil {
		return false
	}
	// Not under TempDir at all.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return false
	}
	// Under TempDir: only treat as transient when the first path segment
	// matches the go toolchain naming convention ("go-build*", "go-run*").
	first := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
	return strings.HasPrefix(first, "go-build") || strings.HasPrefix(first, "go-run")
}

// shellQuote wraps s in single quotes, escaping any embedded single
// quotes, so that it is safe to embed in a POSIX shell command such as
// the git merge.*.driver value.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// goEnvPath returns the value of GOPATH by running "go env GOPATH".
func goEnvPath() (string, error) {
	out, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ensureGitattributes reads .gitattributes, adds any missing
// merge driver entries for the given files, and writes it back.
func ensureGitattributes(path string, files []string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	content := string(existing)

	// Build entries from file list.
	entries := make([]string, len(files))
	for i, f := range files {
		entries[i] = f + " merge=mdsmith"
	}

	// Build a set of normalized existing lines to avoid substring
	// matches against comments or whitespace differences.
	existingSet := make(map[string]struct{})
	for _, line := range strings.Split(content, "\n") {
		norm := strings.Join(strings.Fields(line), " ")
		if norm != "" {
			existingSet[norm] = struct{}{}
		}
	}

	var missing []string
	for _, entry := range entries {
		norm := strings.Join(strings.Fields(entry), " ")
		if _, ok := existingSet[norm]; !ok {
			missing = append(missing, entry)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	// Ensure trailing newline before appending.
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	content += strings.Join(missing, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}
