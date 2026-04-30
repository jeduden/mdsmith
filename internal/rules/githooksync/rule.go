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
//
// The rule is runnable in its zero value: it has no required runtime
// settings, so users can opt in via the bool form `git-hook-sync:
// true`. ApplySettings is still implemented to validate unknown keys
// when the user provides a mapping, but execution does not depend on
// it being called.
type Rule struct {
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

	// Collect drift descriptions from both sources so the rule emits
	// at most one diagnostic per repository per lint run. A blank
	// description from a source means it is in sync (or the user has
	// not opted into that source at all).
	var parts []string
	if msg := r.mergeDriverDrift(repoRoot, discovered); msg != "" {
		parts = append(parts, msg)
	}
	if msg := r.preMergeCommitHookDrift(repoRoot, discovered); msg != "" {
		parts = append(parts, msg)
	}
	if len(parts) == 0 {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  strings.Join(parts, "; "),
	}}
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

// mergeDriverDrift returns a human-readable description of any drift
// between .gitattributes (the real source of truth for which files
// use the mdsmith merge driver) and the discovered file list. The
// check only runs when `merge.mdsmith.driver` is registered, so repos
// that have not opted in are not flagged. Returns an empty string
// when no drift is detected.
//
// An empty `merge=mdsmith` assignment list with the driver registered
// and discovered files present is treated as drift: the merge driver
// will not run for any file, defeating the registration.
func (r *Rule) mergeDriverDrift(repoRoot string, discovered []string) string {
	if !githooks.HasMdsmithMergeDriver(repoRoot) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitattributes"))
	if err != nil && !os.IsNotExist(err) {
		return ""
	}
	installed := githooks.ExtractGitattributesFiles(string(data))
	if githooks.FilesMatch(installed, discovered) {
		return ""
	}
	if len(installed) == 0 {
		return fmt.Sprintf(
			"merge.mdsmith.driver is registered but .gitattributes has no merge=mdsmith entries (should have: %s)",
			strings.Join(discovered, ", "),
		)
	}
	return fmt.Sprintf(
		"merge-driver assignments in .gitattributes are out of sync (has: %s, should have: %s)",
		strings.Join(installed, ", "),
		strings.Join(discovered, ", "),
	)
}

// preMergeCommitHookDrift returns a human-readable description of any
// drift between the installed pre-merge-commit hook and the discovered
// file list. Returns an empty string if no hook is installed, the
// hook is not mdsmith-managed, or the file list matches.
func (r *Rule) preMergeCommitHookDrift(repoRoot string, discovered []string) string {
	hookPath := filepath.Join(githooks.ResolveHooksDir(repoRoot), "pre-merge-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return ""
	}
	hook := string(data)
	if !strings.Contains(hook, githooks.PreMergeCommitMarker) {
		return ""
	}
	installed := githooks.ExtractHookFiles(hook)
	if githooks.FilesMatch(installed, discovered) {
		return ""
	}
	return fmt.Sprintf(
		"pre-merge-commit hook is out of sync (has: %s, should have: %s)",
		strings.Join(installed, ", "),
		strings.Join(discovered, ", "),
	)
}

// ApplySettings implements rule.Configurable. The rule has no runtime
// settings, so this only rejects unknown keys when a user supplies a
// mapping. The rule executes regardless of whether ApplySettings is
// invoked, so a bool-only enable (`git-hook-sync: true`) also works.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k := range settings {
		return fmt.Errorf("git-hook-sync: unknown setting %q", k)
	}
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
