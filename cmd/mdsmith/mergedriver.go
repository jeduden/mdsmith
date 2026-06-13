package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/githooks"
	"github.com/jeduden/mdsmith/internal/rule"
)

const mergeDriverUsage = `Usage: mdsmith merge-driver <subcommand> [args]

Subcommands:
  run <base> <ours> <theirs> <pathname>
        Run as a git custom merge driver. Strips conflict
        markers inside regenerable sections (catalog, include),
        runs mdsmith fix in memory to regenerate them, and exits
        non-zero if unresolved conflict markers remain. Only the
        <ours> temp file is written; the worktree <pathname> is
        never touched, so the parent merge or rebase never sees
        the path as locally modified.

  install [globs...]
        Register the merge driver in git config and ensure
        .gitattributes assigns it. The managed block uses globs
        derived from the project's .mdsmith.yml: include patterns
        (default: *.md and *.markdown) followed by an exclude line
        for each ignore pattern (last-match-wins overrides).

        Optional positional args replace the default include set
        when callers want to scope the merge driver to a custom
        pattern (e.g. docs/**/*.md); .mdsmith.yml ignore
        patterns still apply on top via -merge overrides. Custom
        include globs are not compatible with the MDS048
        git-hook-sync rule's auto-fix, which restores the
        canonical default include set plus ignore-derived
        excludes; do not enable git-hook-sync if you rely on a
        custom include set.

  ci-install
        Like install, but treats the committed .gitattributes as
        the source of truth instead of rewriting it — the npm-ci
        analogue of install's npm-install. It first verifies that
        the committed managed block matches the globs derived from
        .mdsmith.yml, and only then registers the merge driver and
        installs the pre-merge-commit hook (both git-internal and
        untracked). It never writes .gitattributes, and when
        .gitattributes is missing, has no managed block, or has
        drifted it exits before registering the driver or
        installing the hook, so a drift error leaves the
        repository untouched. Use it in CI and the merge queue,
        where rewriting a tracked file mid-run would dirty the
        worktree and abort the merge. Takes no glob arguments; it
        always compares against the canonical default include set,
        so it is incompatible with custom-glob installs (same as
        MDS048).

Git config (set by install / ci-install):
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
	case "ci-install":
		return runMergeDriverCIInstall(args[1:])
	default:
		fmt.Fprintf(os.Stderr,
			"mdsmith: merge-driver: unknown subcommand %q\n\n%s",
			args[0], mergeDriverUsage)
		return 2
	}
}

// mergeFileMode returns the low 9 permission bits (Mode().Perm()) of the named
// file, or defaultMode if the file cannot be stat'd for any reason (including
// ENOENT and permission errors). Uses Lstat so symlinks are not followed.
func mergeFileMode(name string, defaultMode os.FileMode) os.FileMode {
	if info, err := os.Lstat(name); err == nil {
		return info.Mode().Perm()
	}
	return defaultMode
}

// lstatFn is a variable so tests can substitute a failing implementation
// to exercise non-ENOENT Lstat error paths in guardRegularFile.
var lstatFn = os.Lstat

// guardRegularFile returns an error if:
//   - path exists and is not a regular file (symlink, directory, …), or
//   - os.Lstat fails for any reason other than ENOENT.
//
// A missing file (ENOENT) is allowed; the caller decides whether that
// is an error at a higher level.
func guardRegularFile(path string) error {
	info, err := lstatFn(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%s: lstat: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", path)
	}
	return nil
}

// guardFn is a variable so tests can substitute a failing implementation
// to exercise guardRegularFile error paths without creating real symlinks.
var guardFn = guardRegularFile

// osWriteFile is a variable so tests can substitute a failing implementation
// to exercise error paths without needing OS tricks.
var osWriteFile = os.WriteFile

// fixSourceFn is a variable so tests can substitute a failing
// implementation to exercise the fix-failed error path in
// fixMergedContent.
var fixSourceFn = fixMergedSource

// mergeAndClean performs the 3-way merge and strips conflict markers.
// Returns the cleaned content and an exit code (0 on success).
func mergeAndClean(base, ours, theirs string, maxBytes int64) ([]byte, int) {
	// Validate all three inputs before letting git read or write them,
	// so symlinks cannot pull in data from outside the worktree.
	for _, path := range []string{ours, base, theirs} {
		if err := guardFn(path); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return nil, 2
		}
	}
	// Capture the permissions of git's temp file. os.WriteFile preserves
	// the existing mode on truncating writes, so this is only the fallback
	// creation mode for files that do not yet exist.
	oursMode := mergeFileMode(ours, 0o644)

	// Step 1: standard 3-way merge into ours.
	// Use "--" to prevent file paths starting with "-" from being
	// interpreted as git options (option injection).
	mergeCmd := exec.Command("git", "merge-file", "--", ours, base, theirs)
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
	// Re-check before reading to guard against a symlink swap after the merge.
	if err := guardFn(ours); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, 2
	}
	content, err := bytelimit.ReadFileLimited(ours, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading merge result: %v\n", err)
		return nil, 2
	}
	cleaned := stripSectionConflicts(content)
	// Re-check immediately before writing to narrow the TOCTOU window.
	if err := guardFn(ours); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, 2
	}
	if err := osWriteFile(ours, cleaned, oursMode); err != nil {
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

	// Step 3: run mdsmith fix on the merged content in memory and
	// write the result to ours. The worktree path (%P) is never
	// written: the driver runs while the parent merge is still in
	// flight, and even a byte-identical write-and-restore changes
	// the file's stat data, which makes git's up-to-date check
	// treat the path as locally modified and abort the merge with
	// "Your local changes would be overwritten" — under git rebase
	// the pick is rescheduled forever.
	fixed, rc := fixMergedContent(cleaned, ours, pathname, maxBytes)
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

// fixMergedContent runs the mdsmith fix pipeline on cleaned in
// memory — anchored at pathname so config globs and neighbour-file
// reads resolve as they would for the real file — and writes the
// result to ours. pathname itself is never read or written; see
// runMergeDriverRun step 3 for why a worktree write (even one
// restored byte-for-byte) aborts the parent merge.
func fixMergedContent(cleaned []byte, ours, pathname string, maxBytes int64) ([]byte, int) {
	fixed, err := fixSourceFn(pathname, cleaned, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: fix failed: %v\n", err)
		return nil, 2
	}

	// Re-check ours immediately before writing the final merge result.
	if err := guardFn(ours); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, 2
	}
	if err := osWriteFile(ours, fixed, mergeFileMode(ours, 0o644)); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing merge output: %v\n", err)
		return nil, 2
	}
	return fixed, 0
}

// mergeDriverRules is the fix rule set the merge driver runs: every
// registered rule except those that mutate the git index (rules
// implementing rule.GitIndexMutator — today only MDS048,
// git-hook-sync).
//
// git invokes the merge driver from inside `git merge`, which holds
// `.git/index.lock` for the whole merge. A rule whose Fix runs an
// in-process `git add` (e.g. githooks.StageGitattributes) would be a
// second index writer racing the parent `git merge` for that lock,
// which can leave a stale `.git/index.lock` that fails the staging
// step and bounces the merge queue. The merge driver only needs the
// content-regenerating rules to resolve a conflict, so index-mutating
// rules are dropped; the pre-merge-commit hook still runs them
// afterward, when git no longer holds the lock. Filtering by the
// capability interface (not a rule ID) excludes any future
// index-mutating rule automatically.
func mergeDriverRules() []rule.Rule {
	all := rule.All()
	out := make([]rule.Rule, 0, len(all))
	for _, r := range all {
		if m, ok := r.(rule.GitIndexMutator); ok && m.MutatesGitIndex() {
			continue
		}
		out = append(out, r)
	}
	return out
}

// fixMergedSource runs the merge driver's fix rule set over source
// in memory and returns the fixed bytes. path anchors config-glob
// matching, and — because git invokes merge drivers from the
// worktree root — the dirFS derived from it resolves neighbour
// files (include sources, catalog globs) exactly as a fix of the
// on-disk file would. Nothing is written to disk.
func fixMergedSource(path string, source []byte, maxBytes int64) ([]byte, error) {
	cfg, _, err := loadConfig("")
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	return fixpkg.Source(fixpkg.SourceOptions{
		Config:           cfg,
		Rules:            mergeDriverRules(),
		Path:             path,
		Source:           source,
		StripFrontMatter: frontMatterEnabled(cfg),
		MaxInputBytes:    maxBytes,
	})
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

// resolveManagedGlobs returns the merge-driver glob set for an
// install command. With no args, the default include set
// (`*.md`, `*.markdown`) is used and the project's .mdsmith.yml
// `ignore:` patterns become exclude overrides — patterns that
// cannot appear verbatim in `.gitattributes` (whitespace, leading
// `!`) are silently dropped by `GlobsFromConfig`. Explicit args
// replace the include set so callers can scope the merge driver to
// a custom pattern (e.g. `docs/**/*.md`); whitespace in any
// caller-provided include is rejected up front because
// .gitattributes splits attribute lines on whitespace and the bad
// pattern would corrupt the managed block. The second return is
// the process exit code: 0 on success, 2 on a user-facing error
// (already printed to stderr).
func resolveManagedGlobs(_ string, args []string) (githooks.Globs, int) {
	cfg, _, err := loadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: loading config: %v\n", err)
		return githooks.Globs{}, 2
	}
	globs, skipped := githooks.GlobsFromConfig(cfg)
	if len(skipped) > 0 {
		// Surface dropped ignore patterns so operators can see when
		// the generated merge-driver scope diverges from the
		// `ignore:` semantics mdsmith fix itself respects.
		fmt.Fprintf(os.Stderr,
			"mdsmith: skipped unsupported ignore patterns "+
				"(negation or whitespace) when generating "+
				".gitattributes: %s\n",
			strings.Join(skipped, ", "))
	}
	if len(args) > 0 {
		for _, p := range args {
			if strings.ContainsAny(p, " \t\n\r") {
				fmt.Fprintf(os.Stderr,
					"mdsmith: include pattern %q contains whitespace, which is not supported in .gitattributes\n", p)
				return githooks.Globs{}, 2
			}
		}
		globs.Include = append([]string{}, args...)
	}
	return globs, 0
}

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

	globs, rc := resolveManagedGlobs(repoRoot, args)
	if rc != 0 {
		return rc
	}

	attrPath := filepath.Join(repoRoot, ".gitattributes")
	if err := githooks.WriteGitattributes(attrPath, globs); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: updating .gitattributes: %v\n", err)
		return 2
	}

	if err := ensurePreMergeCommitHook(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: installing pre-merge-commit hook: %v\n", err)
		return 2
	}

	hookPath := filepath.Join(resolveHooksDir(repoRoot), "pre-merge-commit")
	fmt.Fprintf(os.Stderr, "mdsmith: merge driver 'mdsmith' installed\n")
	fmt.Fprintf(os.Stderr, "  git config: merge.mdsmith.driver\n")
	fmt.Fprintf(os.Stderr, "  .gitattributes: %s\n", attrPath)
	fmt.Fprintf(os.Stderr, "  pre-merge-commit hook: %s\n", hookPath)
	fmt.Fprintf(os.Stderr,
		"\nTo also enable drift detection, add this to your .mdsmith.yml:\n\n%s\n",
		githooks.EnableRuleSnippet("git-hook-sync"))
	return 0
}

// runMergeDriverCIInstall is the npm-ci analogue of install: it sets up
// the merge machinery (git config driver entry, pre-merge-commit hook)
// but treats the committed .gitattributes as the source of truth rather
// than rewriting it. It verifies that the committed managed block
// matches the glob set derived from .mdsmith.yml and exits non-zero on a
// missing block or drift.
//
// The merge queue runs this instead of install: install re-renders
// .gitattributes from .mdsmith.yml, so a committed copy that has drifted
// gets rewritten, dirtying the worktree and aborting the action's
// `git merge` ("local changes would be overwritten") — which requeues
// the PR and re-fires the labeled trigger forever. ci-install never
// writes the tracked file, so the merge always starts from a clean tree;
// genuine drift surfaces as one clear failed setup step (which does not
// requeue) instead of an infinite loop.
func runMergeDriverCIInstall(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, mergeDriverUsage)
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr,
			"mdsmith: merge-driver ci-install takes no glob arguments; it "+
				"verifies the committed .gitattributes against .mdsmith.yml "+
				"without rewriting it\n")
		return 2
	}

	// Verify we're in a git repo.
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: not in a git repository\n")
		return 2
	}
	repoRoot := strings.TrimSpace(string(out))

	// Compute the expected glob set exactly as install would write it,
	// so the two commands can never disagree on what "in sync" means.
	// nil args means the canonical default include set plus the
	// .mdsmith.yml ignore-derived -merge overrides.
	expected, rc := resolveManagedGlobs(repoRoot, nil)
	if rc != 0 {
		return rc
	}

	// Verify the committed .gitattributes matches — never write it.
	attrPath := filepath.Join(repoRoot, ".gitattributes")
	if rc := verifyGitattributes(attrPath, expected); rc != 0 {
		return rc
	}

	if err := registerMergeDriver(); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	if err := ensurePreMergeCommitHook(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: installing pre-merge-commit hook: %v\n", err)
		return 2
	}

	hookPath := filepath.Join(resolveHooksDir(repoRoot), "pre-merge-commit")
	fmt.Fprintf(os.Stderr, "mdsmith: merge driver 'mdsmith' verified and installed (ci mode)\n")
	fmt.Fprintf(os.Stderr, "  git config: merge.mdsmith.driver\n")
	fmt.Fprintf(os.Stderr, "  .gitattributes: %s (verified in sync; not modified)\n", attrPath)
	fmt.Fprintf(os.Stderr, "  pre-merge-commit hook: %s\n", hookPath)
	return 0
}

// osReadFile is a variable so tests can substitute a failing
// implementation to exercise the read-error path in verifyGitattributes.
var osReadFile = os.ReadFile

// verifyGitattributes checks that the mdsmith managed block in the
// committed .gitattributes at attrPath matches expected, without ever
// writing the file. It returns 0 when they match. On a missing file, a
// missing managed block, or drift it prints a message naming the fix
// (`mdsmith merge-driver install`) and returns 2.
//
// It rejects a symlinked or otherwise non-regular .gitattributes before
// reading, mirroring the lstat guard install applies in
// githooks.WriteGitattributes, to reduce the risk of following a link to
// a path outside the repository. As there, a narrow TOCTOU window
// remains between this guard and the read. A missing file (ENOENT)
// passes the guard and is reported as "not found" below.
func verifyGitattributes(attrPath string, expected githooks.Globs) int {
	if err := guardFn(attrPath); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	data, err := osReadFile(attrPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr,
				"mdsmith: %s not found; run `mdsmith merge-driver install` "+
					"and commit the result\n", attrPath)
			return 2
		}
		fmt.Fprintf(os.Stderr, "mdsmith: reading %s: %v\n", attrPath, err)
		return 2
	}

	installed, ok := githooks.ExtractGlobs(string(data))
	if !ok {
		fmt.Fprintf(os.Stderr,
			"mdsmith: %s has no mdsmith merge-driver managed block; run "+
				"`mdsmith merge-driver install` and commit the result\n",
			attrPath)
		return 2
	}

	if !githooks.GlobsEqual(installed, expected) {
		fmt.Fprintf(os.Stderr,
			"mdsmith: committed .gitattributes is out of sync with "+
				".mdsmith.yml; run `mdsmith merge-driver install` and commit "+
				"the result\n"+
				"  committed: include=%v exclude=%v\n"+
				"  expected:  include=%v exclude=%v\n",
			installed.Include, installed.Exclude,
			expected.Include, expected.Exclude)
		return 2
	}

	return 0
}

// preMergeCommitHookMarker identifies the hook as managed by
// mdsmith so re-running install can safely replace it without
// stomping on a user-authored hook of the same name. The canonical
// constant lives in internal/githooks; this alias keeps existing
// references in this package and its tests stable.
const preMergeCommitHookMarker = githooks.PreMergeCommitMarker

// resolveHooksDir returns the directory where git hooks should be
// installed for the repo at repoRoot. The implementation lives in
// internal/githooks so the CLI and the git-hook-sync rule resolve
// the same path.
func resolveHooksDir(repoRoot string) string {
	return githooks.ResolveHooksDir(repoRoot)
}

// ensurePreMergeCommitHook writes the pre-merge-commit hook so
// that after git resolves all per-file merges (including any
// driver-resolved sections) and before the merge commit is
// created, `mdsmith fix .` runs once over the worktree.
//
// The per-file merge driver cannot do this on its own: when it
// runs on PLAN.md, sibling plan/*.md source files may still hold
// "ours" content because git has not merged them yet, so the
// regenerated catalog reflects a stale view of its sources. The
// hook re-fixes once every path has reached its final merged
// state. The hook content is glob-driven (no per-file list) so it
// stays in sync with .mdsmith.yml ignore patterns automatically.
func ensurePreMergeCommitHook(repoRoot string) error {
	exe, err := resolveInstalledBinary()
	if err != nil {
		return fmt.Errorf("cannot locate mdsmith binary: %w", err)
	}

	hooksDir := resolveHooksDir(repoRoot)
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")

	// Reject symlinks and non-regular files before any I/O to reduce the
	// risk of following a link to a path outside the repository.
	if err := guardFn(hookPath); err != nil {
		return fmt.Errorf("reading existing hook %s: %w", hookPath, err)
	}

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

	content := githooks.BuildHookScript(exe)

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", hooksDir, err)
	}
	// Use atomic temp-then-rename so that even if a symlink is swapped in
	// between the lstat check and the write, os.Rename replaces the
	// directory entry rather than following the link.
	if err := writeHookFile(hookPath, []byte(content)); err != nil {
		return err
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

// chmodFunc is the function used to set file permissions.
// Overridden in tests to exercise the Chmod error path.
var chmodFunc = os.Chmod

// hookCreateTempFn is a variable so tests can substitute a failing
// implementation to exercise the CreateTemp error path in writeHookFile.
var hookCreateTempFn = os.CreateTemp

// osRenameFn is a variable so tests can substitute a failing implementation
// to exercise the Rename error path in writeHookFile.
var osRenameFn = os.Rename

// syncFileFn is a variable so tests can substitute a failing implementation
// to exercise the Sync error path in writeHookFile.
var syncFileFn = (*os.File).Sync

// closeFileFn is a variable so tests can substitute a failing implementation
// to exercise the Close error path in writeHookFile.
var closeFileFn = (*os.File).Close

// writeHookFile writes content to hookPath using a temp-then-rename strategy
// so that os.Rename replaces the directory entry rather than following a
// symlink that may have been introduced between the lstat check in
// ensurePreMergeCommitHook and this call.
func writeHookFile(hookPath string, data []byte) error {
	// Re-check that path is still a regular file (or absent) before writing.
	// guardFn returns nil for ENOENT (file absent is fine — we will create it).
	if err := guardFn(hookPath); err != nil {
		return fmt.Errorf("writing %s: %w", hookPath, err)
	}
	dir := filepath.Dir(hookPath)
	tmp, err := hookCreateTempFn(dir, ".mdsmith-hook-*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", hookPath, err)
	}
	tmpName := tmp.Name()
	// Remove the temp file on any early-exit path. After a successful Rename,
	// tmpName no longer exists so this Remove is a safe no-op.
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = closeFileFn(tmp)
		return fmt.Errorf("writing %s: %w", hookPath, err)
	}
	if err := syncFileFn(tmp); err != nil {
		_ = closeFileFn(tmp)
		return fmt.Errorf("writing %s: %w", hookPath, err)
	}
	if err := closeFileFn(tmp); err != nil {
		return fmt.Errorf("writing %s: %w", hookPath, err)
	}
	if err := chmodFunc(tmpName, 0o755); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", hookPath, err)
	}
	if err := osRenameFn(tmpName, hookPath); err != nil {
		return fmt.Errorf("writing %s: %w", hookPath, err)
	}
	return nil
}

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
