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
  install
        Install the pre-merge-commit hook. After git resolves
        every per-file merge (and runs the mdsmith merge driver
        on configured files) but before creating the merge
        commit, the hook runs 'mdsmith fix .' and stages any
        modified .md/.markdown files. mdsmith fix walks the
        worktree respecting .mdsmith.yml ignore patterns — the
        same set marked with merge=mdsmith in .gitattributes —
        so the hook scope follows the same globbing strategy
        as the .gitattributes managed block.

        Explicit file lists are no longer accepted: scope the
        hook by editing .mdsmith.yml ignore: instead.

  uninstall
        Remove the pre-merge-commit hook if it was installed
        by mdsmith. Refuses to remove user-authored hooks.

  status
        Show whether the hook is installed and whether the
        installed script matches the canonical glob-based
        template.

Git hook installed at:
  .git/hooks/pre-merge-commit (or core.hooksPath if configured)

Example workflow:
  1. Run 'mdsmith pre-merge-commit install' once per clone
  2. During a merge, git runs the hook automatically
  3. The hook runs 'mdsmith fix .' over the worktree
  4. The hook stages modified .md/.markdown files with 'git add'
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

	if len(args) > 0 {
		fmt.Fprintf(os.Stderr,
			"mdsmith: pre-merge-commit install no longer accepts explicit files; "+
				"the hook now runs `mdsmith fix .` and respects "+
				".mdsmith.yml ignore patterns. Edit .mdsmith.yml `ignore:` "+
				"to scope the hook.\n")
		return 2
	}

	if err := ensurePreMergeCommitHook(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: installing pre-merge-commit hook: %v\n", err)
		return 2
	}

	hookPath := filepath.Join(resolveHooksDir(repoRoot), "pre-merge-commit")
	fmt.Fprintf(os.Stderr, "mdsmith: pre-merge-commit hook installed\n")
	fmt.Fprintf(os.Stderr, "  hook path: %s\n", hookPath)
	fmt.Fprintf(os.Stderr, "\nThe hook will run 'mdsmith fix .' during merges and stage\n")
	fmt.Fprintf(os.Stderr, "any modified .md/.markdown files to clear conflict markers.\n")
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
		printManagedHookStatus(repoRoot, content)
	} else {
		fmt.Fprintf(os.Stderr, "  managed by: user (not mdsmith)\n")
		fmt.Fprintf(os.Stderr, "\nThis hook was not installed by mdsmith.\n")
	}

	return 0
}

// printManagedHookStatus prints details for a hook that bears the
// mdsmith marker. The hook content is glob-driven and shared across
// all installs (modulo the binary path), so drift is reported when
// the script differs from the canonical template.
func printManagedHookStatus(_, content string) {
	fmt.Fprintf(os.Stderr, "  managed by: mdsmith\n")
	fmt.Fprintf(os.Stderr, "  scope: `mdsmith fix .` (.mdsmith.yml ignore patterns apply)\n")

	if githooks.HookMatchesCanonical(content) {
		return
	}
	fmt.Fprintf(os.Stderr, "\nWarning: pre-merge-commit hook is out of sync with the glob-based template.\n")
	fmt.Fprintf(os.Stderr, "Run 'mdsmith pre-merge-commit install' to update it.\n")
}
