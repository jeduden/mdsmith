package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/githooks"
)

const preMergeCommitUsage = `Usage: mdsmith pre-merge-commit <subcommand> [args]

Subcommands:
  install [files...]
        Install the pre-merge-commit hook that runs mdsmith fix
        on specified files after git completes all per-file merges
        but before creating the merge commit. This ensures that
        generated sections (catalog, include, toc) reflect the
        final merged state of every source file.

        If files are conflicted, mdsmith fix resolves them and
        the hook stages the resolved files with 'git add' to
        clear the conflict state from the git index.

        When no files are specified, automatically discovers
        files with generated content (catalog, include, toc).

  uninstall
        Remove the pre-merge-commit hook if it was installed
        by mdsmith. Refuses to remove user-authored hooks.

  status
        Show whether the hook is installed and which files it
        processes.

Git hook installed at:
  .git/hooks/pre-merge-commit (or core.hooksPath if configured)

Example workflow:
  1. Run 'mdsmith pre-merge-commit install' once per clone
  2. During a merge, git runs the hook automatically
  3. The hook runs 'mdsmith fix' on each registered file
  4. The hook stages fixed files with 'git add'
  5. Git creates the merge commit with the fixed content
`

// runPreMergeCommit dispatches the pre-merge-commit subcommand.
func runPreMergeCommit(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, preMergeCommitUsage)
		return 0
	}

	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(os.Stderr, preMergeCommitUsage)
		return 0
	case "install":
		return runPreMergeCommitInstall(args[1:])
	case "uninstall":
		return runPreMergeCommitUninstall(args[1:])
	case "status":
		return runPreMergeCommitStatus(args[1:])
	default:
		fmt.Fprintf(os.Stderr,
			"mdsmith: pre-merge-commit: unknown subcommand %q\n\n%s",
			args[0], preMergeCommitUsage)
		return 2
	}
}

// runPreMergeCommitInstall installs the pre-merge-commit hook.
func runPreMergeCommitInstall(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, preMergeCommitUsage)
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

	files, rc := resolveManagedFiles(repoRoot, args)
	if rc != 0 {
		return rc
	}

	if err := ensurePreMergeCommitHook(repoRoot, files); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: installing pre-merge-commit hook: %v\n", err)
		return 2
	}

	hookPath := filepath.Join(resolveHooksDir(repoRoot), "pre-merge-commit")
	fmt.Fprintf(os.Stderr, "mdsmith: pre-merge-commit hook installed\n")
	fmt.Fprintf(os.Stderr, "  hook path: %s\n", hookPath)
	fmt.Fprintf(os.Stderr, "  files: %s\n", strings.Join(files, ", "))
	fmt.Fprintf(os.Stderr, "\nThe hook will run 'mdsmith fix' on these files during merges\n")
	fmt.Fprintf(os.Stderr, "and stage them with 'git add' to clear any conflict markers.\n")
	fmt.Fprintf(os.Stderr,
		"\nTo also enable drift detection, add this to your .mdsmith.yml:\n\n%s\n",
		githooks.EnableRuleSnippet("git-hook-sync"))
	return 0
}

// runPreMergeCommitUninstall removes the pre-merge-commit hook.
func runPreMergeCommitUninstall(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, preMergeCommitUsage)
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

	hooksDir := resolveHooksDir(repoRoot)
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")

	// Check if hook exists.
	existing, readErr := os.ReadFile(hookPath)
	if os.IsNotExist(readErr) {
		fmt.Fprintf(os.Stderr, "mdsmith: no pre-merge-commit hook found at %s\n", hookPath)
		return 0
	}
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading hook: %v\n", readErr)
		return 2
	}

	// Refuse to remove a hook we didn't create.
	if !strings.Contains(string(existing), preMergeCommitHookMarker) {
		fmt.Fprintf(os.Stderr,
			"mdsmith: %s exists but was not installed by mdsmith\n"+
				"Remove it manually if you want to delete it.\n",
			hookPath)
		return 2
	}

	// Remove the hook.
	if err := os.Remove(hookPath); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: removing hook: %v\n", err)
		return 2
	}

	fmt.Fprintf(os.Stderr, "mdsmith: pre-merge-commit hook removed from %s\n", hookPath)
	return 0
}

// runPreMergeCommitStatus shows the current hook installation status.
func runPreMergeCommitStatus(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, preMergeCommitUsage)
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

	hooksDir := resolveHooksDir(repoRoot)
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")

	// Check if hook exists.
	existing, readErr := os.ReadFile(hookPath)
	if os.IsNotExist(readErr) {
		fmt.Fprintf(os.Stderr, "pre-merge-commit hook: not installed\n")
		fmt.Fprintf(os.Stderr, "  expected path: %s\n", hookPath)
		fmt.Fprintf(os.Stderr, "\nRun 'mdsmith pre-merge-commit install' to install it.\n")
		return 1
	}
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading hook: %v\n", readErr)
		return 2
	}

	// Check if it's our hook.
	content := string(existing)
	managed := strings.Contains(content, preMergeCommitHookMarker)

	fmt.Fprintf(os.Stderr, "pre-merge-commit hook: installed\n")
	fmt.Fprintf(os.Stderr, "  path: %s\n", hookPath)
	if managed {
		fmt.Fprintf(os.Stderr, "  managed by: mdsmith\n")

		// Extract file list from hook content.
		files := extractFilesFromHook(content)
		if len(files) > 0 {
			fmt.Fprintf(os.Stderr, "  files: %s\n", strings.Join(files, ", "))
		}

		// Check if hook files are in sync with discovered files.
		cfg, _, err := loadConfig("")
		if err == nil {
			maxBytes, err := resolveMaxInputBytes(cfg, "")
			if err == nil {
				discoveredFiles := discoverFilesWithGeneratedContent(repoRoot, maxBytes)
				if !filesMatch(files, discoveredFiles) {
					fmt.Fprintf(os.Stderr, "\nWarning: hook files are out of sync with repository\n")
					fmt.Fprintf(os.Stderr, "  discovered files: %s\n", strings.Join(discoveredFiles, ", "))
					fmt.Fprintf(os.Stderr, "\nRun 'mdsmith pre-merge-commit install' to update the hook.\n")
				}
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "  managed by: user (not mdsmith)\n")
		fmt.Fprintf(os.Stderr, "\nThis hook was not installed by mdsmith.\n")
	}

	return 0
}

// extractFilesFromHook parses the hook content to extract the list
// of files it processes. Implementation lives in internal/githooks so
// the CLI and the git-hook-sync rule cannot drift.
func extractFilesFromHook(content string) []string {
	return githooks.ExtractHookFiles(content)
}

// filesMatch reports whether a and b contain the same set of files,
// regardless of order. Implementation lives in internal/githooks.
func filesMatch(a, b []string) bool {
	return githooks.FilesMatch(a, b)
}
