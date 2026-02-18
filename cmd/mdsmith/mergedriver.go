package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

const mergeDriverUsage = `Usage: mdsmith merge-driver <subcommand> [args]

Subcommands:
  run <base> <ours> <theirs> <pathname>
        Run as a git custom merge driver. Strips conflict
        markers inside catalog blocks, runs mdsmith fix to
        regenerate them, and exits non-zero if unresolved
        conflict markers remain.

  install
        Register the merge driver in git config and ensure
        .gitattributes assigns it to catalog files.

Git config (set by install):
  merge.catalog.driver = mdsmith merge-driver run %O %A %B %P
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
	mergeErr := mergeCmd.Run()

	// Step 2: strip conflict markers inside catalog sections.
	content, err := os.ReadFile(ours)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading merge result: %v\n", err)
		return 2
	}

	cleaned := stripCatalogConflicts(content)
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
		if mergeErr != nil {
			fmt.Fprintf(os.Stderr,
				"mdsmith: unresolved conflict markers remain in %s\n",
				pathname)
		}
		return 1
	}

	return 0
}

// fixAtRealPath writes cleaned content to pathname, runs mdsmith
// fix, copies the result to ours, and restores pathname.
func fixAtRealPath(cleaned []byte, ours, pathname string) ([]byte, int) {
	backup, backupErr := os.ReadFile(pathname)
	if err := os.WriteFile(pathname, cleaned, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing to %s: %v\n", pathname, err)
		return nil, 2
	}

	fixErr := fixFileInPlace(pathname)

	fixed, err := os.ReadFile(pathname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading fixed file: %v\n", err)
		return nil, 2
	}

	if err := os.WriteFile(ours, fixed, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing merge output: %v\n", err)
		return nil, 2
	}

	// Restore the original working tree file. Git will overwrite
	// it with the merge result from ours after the driver exits.
	if backupErr == nil {
		_ = os.WriteFile(pathname, backup, 0644)
	}

	if fixErr != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: fix failed: %v\n", fixErr)
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

// stripCatalogConflicts removes git conflict markers (<<<<<<,
// ======, >>>>>>) from lines that fall inside catalog sections
// delimited by <!-- catalog --> and <!-- /catalog -->. Conflict
// markers outside catalog sections are left unchanged.
func stripCatalogConflicts(content []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	var out [][]byte
	inCatalog := false

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)

		if bytes.HasPrefix(trimmed, []byte("<!-- catalog")) {
			inCatalog = true
		}
		if bytes.Equal(trimmed, []byte("<!-- /catalog -->")) {
			inCatalog = false
		}

		if inCatalog && isConflictMarker(trimmed) {
			continue
		}

		out = append(out, line)
	}

	return bytes.Join(out, []byte("\n"))
}

// isConflictMarker returns true if the line is a git conflict
// marker (starts with <<<<<<<, =======, or >>>>>>>).
func isConflictMarker(line []byte) bool {
	return bytes.HasPrefix(line, []byte("<<<<<<<")) ||
		bytes.HasPrefix(line, []byte("=======")) ||
		bytes.HasPrefix(line, []byte(">>>>>>>"))
}

// hasConflictMarkers returns true if the content contains any
// git conflict markers.
func hasConflictMarkers(content []byte) bool {
	for _, line := range bytes.Split(content, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if isConflictMarker(trimmed) {
			return true
		}
	}
	return false
}

// runMergeDriverInstall registers the catalog merge driver in
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

	// Register merge driver in git config.
	driver := "mdsmith merge-driver run %O %A %B %P"
	cmds := [][]string{
		{"git", "config", "merge.catalog.name",
			"Catalog-aware Markdown merge"},
		{"git", "config", "merge.catalog.driver", driver},
	}
	for _, c := range cmds {
		if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: git config failed: %v\n", err)
			return 2
		}
	}

	// Ensure .gitattributes has the merge driver entries.
	attrPath := repoRoot + "/.gitattributes"
	if err := ensureGitattributes(attrPath); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: updating .gitattributes: %v\n", err)
		return 2
	}

	fmt.Fprintf(os.Stderr, "mdsmith: merge driver 'catalog' installed\n")
	fmt.Fprintf(os.Stderr, "  git config: merge.catalog.driver\n")
	fmt.Fprintf(os.Stderr, "  .gitattributes: %s\n", attrPath)
	return 0
}

// catalogAttrEntries are the .gitattributes lines the install
// command ensures exist.
var catalogAttrEntries = []string{
	"PLAN.md merge=catalog",
	"README.md merge=catalog",
}

// ensureGitattributes reads .gitattributes, adds any missing
// catalog merge driver entries, and writes it back.
func ensureGitattributes(path string) error {
	existing, _ := os.ReadFile(path)
	content := string(existing)

	var missing []string
	for _, entry := range catalogAttrEntries {
		if !strings.Contains(content, entry) {
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
