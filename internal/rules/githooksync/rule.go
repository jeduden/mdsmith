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
//
// "At most one diagnostic per repository" is enforced via package-
// level state rather than per-Rule state, because the engine clones
// Configurable rules per file when the rule is enabled with a
// settings mapping (even an empty `{}`). With per-instance state,
// each clone would re-emit the diagnostic for every file in the
// repo.
type Rule struct{}

// reportedRepos tracks repositories already reported against during
// the lifetime of this process, guarded by reportedMu. The set lives
// at package scope so per-file Rule clones share the same state.
var (
	reportedMu    sync.Mutex
	reportedRepos = make(map[string]bool)
)

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
	// Skip when there is no on-disk file to anchor repo discovery.
	// stdin and other in-memory inputs have f.FS == nil and a
	// synthetic f.Path like "<stdin>"; if we used filepath.Dir on
	// that, the rule would scan whatever git repo happens to be
	// the process working directory and emit drift unrelated to
	// the content being linted.
	if f.FS == nil {
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

	// Cheap opt-in probes before the (expensive) repo walk. The
	// merge-driver source only applies when the local config
	// registers `merge.mdsmith.driver`, and the hook source only
	// applies when an mdsmith-marked pre-merge-commit hook is
	// installed. If neither is opted in (and the hook is not
	// unreadable, which would still warrant a warning) there is
	// nothing to compare against, so skip the discovery walk.
	hasDriver := githooks.HasMdsmithMergeDriver(repoRoot)
	hookState := peekHookSource(repoRoot)
	if !hasDriver && hookState != hookSourceManaged && hookState != hookSourceUnreadable {
		return nil
	}

	discovered := githooks.DiscoverFiles(repoRoot, f.MaxInputBytes)

	// Collect drift descriptions from both sources so the rule emits
	// at most one diagnostic per repository during the lifetime of
	// this process (the reportedRepos guard is process-scoped, not
	// run-scoped — see markReported). A blank description from a
	// source means it is in sync (or the user has not opted into
	// that source at all).
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

// hookSource describes the state of the pre-merge-commit hook for
// the cheap pre-check in Check. Distinguishing "not installed"
// (ENOENT) from "couldn't read" lets the rule still surface IO
// errors via preMergeCommitHookDrift even when the merge driver
// isn't registered.
type hookSource int

const (
	hookSourceAbsent hookSource = iota
	hookSourceManaged
	hookSourceUnmanaged
	hookSourceUnreadable
)

// peekHookSource reports the current state of the pre-merge-commit
// hook without parsing its file list. It is a cheap probe used by
// Check to decide whether the (expensive) repo discovery walk is
// needed at all.
func peekHookSource(repoRoot string) hookSource {
	hookPath := filepath.Join(githooks.ResolveHooksDir(repoRoot), "pre-merge-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return hookSourceAbsent
		}
		return hookSourceUnreadable
	}
	if strings.Contains(string(data), githooks.PreMergeCommitMarker) {
		return hookSourceManaged
	}
	return hookSourceUnmanaged
}

// markReported returns true exactly once per repoRoot for the
// lifetime of the process. Subsequent calls for the same repo return
// false so duplicate diagnostics are not emitted while linting many
// files in the same repo. Shared via package-level state so the
// guarantee holds even when the engine clones the rule per file.
func (r *Rule) markReported(repoRoot string) bool {
	reportedMu.Lock()
	defer reportedMu.Unlock()
	if reportedRepos[repoRoot] {
		return false
	}
	reportedRepos[repoRoot] = true
	return true
}

// resetReportedForTest clears the package-level reported set. Tests
// call it via t.Cleanup so independent cases do not leak state into
// each other.
func resetReportedForTest() {
	reportedMu.Lock()
	defer reportedMu.Unlock()
	reportedRepos = make(map[string]bool)
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
// will not run for any file, defeating the registration. A non-ENOENT
// read error is surfaced as drift too rather than silently passing,
// so permission/IO failures cannot mask real misconfiguration.
func (r *Rule) mergeDriverDrift(repoRoot string, discovered []string) string {
	if !githooks.HasMdsmithMergeDriver(repoRoot) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitattributes"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Sprintf(
			"cannot verify merge-driver assignments because .gitattributes could not be read: %v",
			err,
		)
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
	shouldDesc := strings.Join(discovered, ", ")
	if len(discovered) == 0 {
		shouldDesc = "(none)"
	}
	return fmt.Sprintf(
		"merge-driver assignments in .gitattributes are out of sync (has: %s, should have: %s)",
		strings.Join(installed, ", "),
		shouldDesc,
	)
}

// preMergeCommitHookDrift returns a human-readable description of any
// drift between the installed pre-merge-commit hook and the discovered
// file list. Returns an empty string if no hook is installed, the
// hook is not mdsmith-managed, or the file list matches. A non-ENOENT
// read error is surfaced rather than silently passing so permission
// or IO failures cannot mask real drift.
func (r *Rule) preMergeCommitHookDrift(repoRoot string, discovered []string) string {
	hookPath := filepath.Join(githooks.ResolveHooksDir(repoRoot), "pre-merge-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return fmt.Sprintf(
			"cannot verify pre-merge-commit hook because %s could not be read: %v",
			hookPath, err,
		)
	}
	hook := string(data)
	if !strings.Contains(hook, githooks.PreMergeCommitMarker) {
		return ""
	}
	installed := githooks.ExtractHookFiles(hook)
	if githooks.FilesMatch(installed, discovered) {
		return ""
	}
	hasDesc := strings.Join(installed, ", ")
	if len(installed) == 0 {
		hasDesc = "(none)"
	}
	shouldDesc := strings.Join(discovered, ", ")
	if len(discovered) == 0 {
		shouldDesc = "(none)"
	}
	return fmt.Sprintf(
		"pre-merge-commit hook is out of sync (has: %s, should have: %s)",
		hasDesc,
		shouldDesc,
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
