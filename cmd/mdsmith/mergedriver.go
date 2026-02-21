package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	fixpkg "github.com/jeduden/mdsmith/internal/fix"
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
  merge.mdsmith.driver = mdsmith merge-driver run %O %A %B %P
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

	// Step 1: standard 3-way merge into ours.
	mergeCmd := exec.Command("git", "merge-file", ours, base, theirs)
	mergeCmd.Stderr = os.Stderr
	mergeErr := mergeCmd.Run()

	// git merge-file exits 1 for conflicts, 2+ for fatal errors.
	// Non-ExitError (e.g. git not found) is also fatal.
	if mergeErr != nil {
		if exitErr, ok := mergeErr.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			fmt.Fprintf(os.Stderr, "mdsmith: git merge-file failed: %v\n", mergeErr)
			return 2
		}
	}

	// Step 2: strip conflict markers inside regenerable sections.
	content, err := os.ReadFile(ours)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading merge result: %v\n", err)
		return 2
	}

	cleaned := stripSectionConflicts(content)
	if err := os.WriteFile(ours, cleaned, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing cleaned merge: %v\n", err)
		return 2
	}

	// Step 3: run mdsmith fix at the real path and write result
	// back to ours.
	fixed, rc := fixAtRealPath(cleaned, ours, pathname)
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
func fixAtRealPath(cleaned []byte, ours, pathname string) ([]byte, int) {
	// Capture the original file mode so we can preserve permissions.
	fileMode := os.FileMode(0644)
	if info, err := os.Stat(pathname); err == nil {
		fileMode = info.Mode()
	}

	backup, backupErr := os.ReadFile(pathname)
	if backupErr != nil && !os.IsNotExist(backupErr) {
		fmt.Fprintf(os.Stderr, "mdsmith: reading %s for backup: %v\n", pathname, backupErr)
		return nil, 2
	}
	if err := os.WriteFile(pathname, cleaned, fileMode); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing to %s: %v\n", pathname, err)
		return nil, 2
	}

	fixErr := fixFileInPlace(pathname)

	// Restore the original working tree file before checking
	// fixErr, so the working tree is always left clean.
	fixed, err := os.ReadFile(pathname)
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
func fixFileInPlace(path string) error {
	cfg, _, err := loadConfig("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           &vlog.Logger{},
	}

	result := fixer.Fix([]string{path})
	if len(result.Errors) > 0 {
		return result.Errors[0]
	}
	return nil
}

// regenSectionNames lists the section types whose content is
// regenerated by mdsmith fix. Conflict markers inside these
// sections can be safely stripped before regeneration.
var regenSectionNames = []string{"catalog", "include"}

// isRegenSectionStart returns true if the line starts a
// regenerable section. The start marker begins with
// "<!-- catalog" or "<!-- include", followed by a YAML body
// and a closing "-->".
func isRegenSectionStart(line []byte) bool {
	for _, name := range regenSectionNames {
		if bytes.HasPrefix(line, []byte("<!-- "+name)) {
			return true
		}
	}
	return false
}

// isRegenSectionEnd returns true if the line ends a regenerable
// section (e.g. <!-- /catalog --> or <!-- /include -->).
func isRegenSectionEnd(line []byte) bool {
	for _, name := range regenSectionNames {
		if bytes.Equal(line, []byte("<!-- /"+name+" -->")) {
			return true
		}
	}
	return false
}

// stripSectionConflicts removes git conflict markers from lines
// that fall inside regenerable sections (catalog, include).
// Conflict markers outside these sections are left unchanged.
//
// The ======= separator is only stripped when it appears between
// <<<<<<< and >>>>>>> to avoid false positives with Markdown
// setext heading underlines.
func stripSectionConflicts(content []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	var out [][]byte
	inSection := false
	inConflict := false

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)

		if isRegenSectionStart(trimmed) {
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
			if inConflict && isConflictSeparator(trimmed) {
				continue
			}
		}

		out = append(out, line)

		if isRegenSectionEnd(trimmed) {
			inSection = false
			inConflict = false
		}
	}

	return bytes.Join(out, []byte("\n"))
}

// isConflictOpen returns true if the line opens a git conflict
// block (starts with <<<<<<<).
func isConflictOpen(line []byte) bool {
	return bytes.HasPrefix(line, []byte("<<<<<<<"))
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

	fmt.Fprintf(os.Stderr, "mdsmith: merge driver 'mdsmith' installed\n")
	fmt.Fprintf(os.Stderr, "  git config: merge.mdsmith.driver\n")
	fmt.Fprintf(os.Stderr, "  .gitattributes: %s\n", attrPath)
	return 0
}

// registerMergeDriver writes the merge.mdsmith.* keys to local
// git config.
func registerMergeDriver() error {
	driver := "mdsmith merge-driver run %O %A %B %P"
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
