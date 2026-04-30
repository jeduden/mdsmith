package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const preMergeCommitUsage = `Usage: mdsmith pre-merge-commit <subcommand> [args]

Subcommands:
  install [files...]
        Install the pre-merge-commit hook that runs mdsmith fix
        on specified files after git completes all per-file merges
        but before creating the merge commit. This ensures that
        generated sections (catalog, include) reflect the final
        merged state of every source file.

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

	// Determine file list: use args if given, else discover files
	// with generated content.
	var files []string
	if len(args) > 0 {
		files = args
	} else {
		// Resolve max input size from config.
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
		files = discoverFilesWithGeneratedContent(repoRoot, maxBytes)
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
					fmt.Fprintf(os.Stderr, "\n⚠ Warning: hook files are out of sync with repository\n")
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
// of files it processes. Returns nil if parsing fails.
func extractFilesFromHook(content string) []string {
	// Look for lines like: '/path/to/mdsmith' fix -- 'FILE.md'
	// This is a simple heuristic extraction.
	var files []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "fix --") {
			continue
		}
		// Extract the file after 'fix --'
		parts := strings.Split(line, "fix --")
		if len(parts) < 2 {
			continue
		}
		// The file is the last argument, typically in single quotes.
		after := strings.TrimSpace(parts[1])
		// Remove trailing 'git add' command if present on same line
		if idx := strings.Index(after, "\n"); idx != -1 {
			after = after[:idx]
		}
		// Strip quotes and extract filename.
		after = strings.Trim(after, "' \t")
		if after != "" && !strings.Contains(after, "fi") {
			files = append(files, after)
		}
	}
	return files
}

// filesMatch checks if two file lists contain the same files,
// regardless of order. Returns true if they match, false otherwise.
func filesMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// Create a map to track files in list a.
	filesInA := make(map[string]bool)
	for _, f := range a {
		filesInA[f] = true
	}

	// Check that all files in b are also in a.
	for _, f := range b {
		if !filesInA[f] {
			return false
		}
	}

	return true
}
