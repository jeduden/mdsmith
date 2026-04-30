package githooksync

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that git hooks (merge-driver and pre-merge-commit) are in sync
// with the files that contain generated content directives.
type Rule struct {
	configured bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS048" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "git-hook-sync" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if !r.configured {
		return nil
	}

	// Find git repo root.
	repoRoot, err := getGitRepoRoot()
	if err != nil {
		// Not in a git repo or git not available - skip check.
		return nil
	}

	// Only check once per repository, not for every file.
	// We use a heuristic: only check when processing README.md or the first
	// file alphabetically to avoid reporting the same issue N times.
	if !shouldCheckHooks(f.Path) {
		return nil
	}

	// Discover files with generated content.
	discoveredFiles := discoverFilesWithGeneratedContent(repoRoot, f.MaxInputBytes)

	var diags []lint.Diagnostic

	// Check merge-driver hook.
	mergeDriverPath := filepath.Join(repoRoot, ".git", "config")
	if content, err := os.ReadFile(mergeDriverPath); err == nil {
		installedFiles := extractMergeDriverFiles(string(content))
		if len(installedFiles) > 0 && !filesMatch(installedFiles, discoveredFiles) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     1,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf(
					"merge-driver hook is out of sync (has: %s, should have: %s)",
					strings.Join(installedFiles, ", "),
					strings.Join(discoveredFiles, ", "),
				),
			})
		}
	}

	// Check pre-merge-commit hook.
	hooksDir := resolveHooksDir(repoRoot)
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	if content, err := os.ReadFile(hookPath); err == nil {
		hookContent := string(content)
		if strings.Contains(hookContent, "# mdsmith pre-merge-commit hook") {
			installedFiles := extractHookFiles(hookContent)
			if len(installedFiles) > 0 && !filesMatch(installedFiles, discoveredFiles) {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     1,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message: fmt.Sprintf(
						"pre-merge-commit hook is out of sync (has: %s, should have: %s)",
						strings.Join(installedFiles, ", "),
						strings.Join(discoveredFiles, ", "),
					),
				})
			}
		}
	}

	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	// The fix is to run the install commands, which mdsmith fix cannot do
	// directly. Instead, we return the original content unchanged and rely
	// on the user to run the install commands manually.
	// In the future, we could make this smarter by actually calling the
	// install logic, but for now we just report the issue.
	return f.Source
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k := range settings {
		return fmt.Errorf("git-hook-sync: unknown setting %q", k)
	}
	r.configured = true
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{}
}

// shouldCheckHooks returns true if we should check hooks for this file.
// We only check once per repository to avoid duplicate diagnostics.
func shouldCheckHooks(path string) bool {
	// Check for README.md or PLAN.md as a proxy for "first file"
	base := filepath.Base(path)
	return base == "README.md" || base == "PLAN.md"
}

// getGitRepoRoot returns the root directory of the git repository.
func getGitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveHooksDir returns the hooks directory for the repository.
func resolveHooksDir(repoRoot string) string {
	// Check if core.hooksPath is configured.
	cmd := exec.Command("git", "config", "core.hooksPath")
	cmd.Dir = repoRoot
	if out, err := cmd.Output(); err == nil {
		custom := strings.TrimSpace(string(out))
		if custom != "" {
			if filepath.IsAbs(custom) {
				return custom
			}
			return filepath.Join(repoRoot, custom)
		}
	}
	return filepath.Join(repoRoot, ".git", "hooks")
}

// discoverFilesWithGeneratedContent scans the repository for markdown
// files containing generated section directives (catalog, include, toc).
// Returns a list of file paths relative to repoRoot, or falls back to
// sensible defaults if discovery fails or finds no files.
func discoverFilesWithGeneratedContent(repoRoot string, maxBytes int64) []string {
	var filesWithDirectives []string

	// Get directive names from registered rules.
	directiveNames := make(map[string]bool)
	for _, r := range rule.All() {
		if d, ok := r.(gensection.Directive); ok {
			directiveNames[d.Name()] = true
		}
	}

	// Walk the repository looking for markdown files.
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		// Skip non-files, hidden directories, and non-markdown files.
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(info.Name(), ".md") && !strings.HasSuffix(info.Name(), ".markdown") {
			return nil
		}

		// Read file and check for directives.
		content, err := lint.ReadFileLimited(path, maxBytes)
		if err != nil {
			return nil // Skip files we can't read
		}

		// Check if file contains any directive markers.
		hasDirective := false
		for name := range directiveNames {
			marker := []byte("<?" + name)
			if bytes.Contains(content, marker) {
				hasDirective = true
				break
			}
		}

		if hasDirective {
			// Convert to relative path from repo root.
			relPath, err := filepath.Rel(repoRoot, path)
			if err == nil {
				filesWithDirectives = append(filesWithDirectives, relPath)
			}
		}

		return nil
	})

	// If discovery failed or found nothing, return sensible defaults.
	if err != nil || len(filesWithDirectives) == 0 {
		return []string{"PLAN.md", "README.md"}
	}

	return filesWithDirectives
}

// extractMergeDriverFiles extracts the file list from .git/config.
func extractMergeDriverFiles(gitConfig string) []string {
	var files []string
	inMergeSection := false
	for _, line := range strings.Split(gitConfig, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[merge \"mdsmith-") {
			inMergeSection = true
			continue
		}
		if inMergeSection {
			if strings.HasPrefix(trimmed, "[") {
				inMergeSection = false
				continue
			}
			if strings.HasPrefix(trimmed, "driver =") {
				// Extract file from driver command
				// Format: driver = mdsmith merge-driver -- 'FILE.md' %O %A %B %P
				parts := strings.Split(trimmed, "merge-driver -- ")
				if len(parts) >= 2 {
					filePart := strings.Split(parts[1], " ")[0]
					filePart = strings.Trim(filePart, "'\"")
					if filePart != "" {
						files = append(files, filePart)
					}
				}
			}
		}
	}
	return files
}

// extractHookFiles extracts the file list from pre-merge-commit hook.
func extractHookFiles(hookContent string) []string {
	var files []string
	for _, line := range strings.Split(hookContent, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "fix --") {
			continue
		}
		parts := strings.Split(line, "fix --")
		if len(parts) < 2 {
			continue
		}
		after := strings.TrimSpace(parts[1])
		after = strings.Trim(after, "' \t")
		if after != "" && !strings.Contains(after, "fi") {
			files = append(files, after)
		}
	}
	return files
}

// filesMatch checks if two file lists contain the same files,
// regardless of order.
func filesMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	filesInA := make(map[string]bool)
	for _, f := range a {
		filesInA[f] = true
	}
	for _, f := range b {
		if !filesInA[f] {
			return false
		}
	}
	return true
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
)
