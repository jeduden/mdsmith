// Package githooksync implements MDS048, the git-hook-sync rule. It
// reports when the pre-merge-commit hook or .gitattributes merge
// driver assignments do not list the same files that currently
// contain generated-section directives.
package githooksync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/githooks"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that mdsmith-managed git hooks and .gitattributes are
// in sync with the files that contain generated-section directives.
type Rule struct {
	configured bool

	// reportedMu guards reported.
	reportedMu sync.Mutex
	// reported tracks repositories already reported against during
	// this lint run so the rule emits at most one diagnostic per
	// repository regardless of which file triggered the check.
	reported map[string]bool
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

	// Resolve the repo root from the directory of the file being
	// linted so the rule does not depend on the process working
	// directory. When a file is not inside a git repo, skip silently.
	repoRoot, err := githooks.GitRepoRoot(filepath.Dir(f.Path))
	if err != nil {
		return nil
	}

	if !r.markReported(repoRoot) {
		return nil
	}

	discovered := githooks.DiscoverFiles(repoRoot, f.MaxInputBytes)

	var diags []lint.Diagnostic
	diags = append(diags, r.checkMergeDriverAttrs(f, repoRoot, discovered)...)
	diags = append(diags, r.checkPreMergeCommitHook(f, repoRoot, discovered)...)
	return diags
}

// markReported returns true exactly once per repoRoot for the lifetime
// of this Rule instance. Subsequent calls for the same repo return
// false so duplicate diagnostics are not emitted while linting many
// files in the same repo.
func (r *Rule) markReported(repoRoot string) bool {
	r.reportedMu.Lock()
	defer r.reportedMu.Unlock()
	if r.reported == nil {
		r.reported = make(map[string]bool)
	}
	if r.reported[repoRoot] {
		return false
	}
	r.reported[repoRoot] = true
	return true
}

// checkMergeDriverAttrs reads .gitattributes (the real source of truth
// for which files use the mdsmith merge driver) and reports drift
// against the discovered file list. The check only runs when
// `merge.mdsmith.driver` is registered, so repos that have not opted
// in are not flagged.
func (r *Rule) checkMergeDriverAttrs(f *lint.File, repoRoot string, discovered []string) []lint.Diagnostic {
	if !githooks.HasMdsmithMergeDriver(repoRoot) {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitattributes"))
	if err != nil {
		return nil
	}
	installed := githooks.ExtractGitattributesFiles(string(data))
	if len(installed) == 0 || githooks.FilesMatch(installed, discovered) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"merge-driver assignments in .gitattributes are out of sync (has: %s, should have: %s)",
			strings.Join(installed, ", "),
			strings.Join(discovered, ", "),
		),
	}}
}

// checkPreMergeCommitHook reads the pre-merge-commit hook (if any)
// installed by mdsmith and reports drift against the discovered file
// list. Hooks not bearing the mdsmith marker are left alone.
func (r *Rule) checkPreMergeCommitHook(f *lint.File, repoRoot string, discovered []string) []lint.Diagnostic {
	hookPath := filepath.Join(githooks.ResolveHooksDir(repoRoot), "pre-merge-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return nil
	}
	hook := string(data)
	if !strings.Contains(hook, githooks.PreMergeCommitMarker) {
		return nil
	}
	installed := githooks.ExtractHookFiles(hook)
	if len(installed) == 0 || githooks.FilesMatch(installed, discovered) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"pre-merge-commit hook is out of sync (has: %s, should have: %s)",
			strings.Join(installed, ", "),
			strings.Join(discovered, ", "),
		),
	}}
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

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
