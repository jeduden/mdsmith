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
type Rule struct{}

// discoveredCache stores the result of DiscoverFiles per repo to avoid
// re-scanning the repo for every file. Discovery is expensive (full
// repo walk + file reads), so caching it significantly improves
// performance when checking many files in the same repo. The cache
// key includes maxBytes because DiscoverFiles uses that limit when
// reading each candidate.
//
// stagingErrors records repos where Fix wrote .gitattributes but the
// follow-up `git add -- .gitattributes` failed (e.g. index.lock
// contention). The on-disk fix already happened, so a plain drift
// re-check would see the file as in sync and stop emitting
// diagnostics — silently leaving the staged tree out of sync with
// the working tree. Surfacing the failure through Check makes it
// retryable: subsequent Fix calls re-run the staging step until it
// succeeds, at which point the entry is cleared.
// repoRootCache memoises the result of GitRepoRoot(dir) so per-file
// Check/Fix calls do not respawn `git rev-parse --show-toplevel` for
// every file. Entries with a non-nil error are also cached so
// non-repo directories are remembered too. Keyed by the directory
// passed to resolveRepoRoot, not the resolved root, because two
// directories under the same repo still produce one git invocation.
var (
	discoveredMu    sync.Mutex
	discoveredCache = make(map[string][]string)
	stagingMu       sync.Mutex
	stagingErrors   = make(map[string]error)
	repoRootMu      sync.Mutex
	repoRootCache   = make(map[string]repoRootEntry)
)

type repoRootEntry struct {
	root string
	err  error
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
	// The lookup result is memoised per directory so linting many
	// files in the same repo does not respawn `git rev-parse` for
	// each one.
	repoRoot, err := r.resolveRepoRoot(filepath.Dir(f.Path))
	if err != nil {
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

	// Discovery is cached per repo and only consumed by the drift
	// functions once they get past their own IO short-circuits.
	// Wrap it in a lazy getter so the (potentially expensive) walk
	// is skipped entirely on the unreadable-only path: when the user
	// has not registered the merge driver and the hook file is
	// unreadable, preMergeCommitHookDrift returns the IO diagnostic
	// before reaching the FilesMatch check, and the walk is wasted.
	var (
		discovered    []string
		haveDiscovery bool
	)
	getDiscovered := func() []string {
		if !haveDiscovery {
			discovered = r.getDiscovered(repoRoot, f.MaxInputBytes)
			haveDiscovery = true
		}
		return discovered
	}

	// Collect drift descriptions from both sources. A blank
	// description from a source means it is in sync (or the user
	// has not opted into that source at all).
	var parts []string
	if msg := r.mergeDriverDrift(repoRoot, hasDriver, getDiscovered); msg != "" {
		parts = append(parts, msg)
	}
	if msg := r.preMergeCommitHookDrift(repoRoot, getDiscovered); msg != "" {
		parts = append(parts, msg)
	}
	// A previous Fix may have written .gitattributes but failed to
	// stage it. Surface that as a diagnostic so the next Fix call is
	// triggered to retry the staging.
	if err := stagingError(repoRoot); err != nil {
		parts = append(parts, fmt.Sprintf(
			".gitattributes was regenerated but `git add` failed: %v "+
				"(run `git add -- .gitattributes` or re-run mdsmith fix to retry)",
			err,
		))
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

// mergeDriverDrift returns a human-readable description of any drift
// between .gitattributes (the real source of truth for which files
// use the mdsmith merge driver) and the discovered file list. The
// check only runs when `merge.mdsmith.driver` is registered, so repos
// that have not opted in are not flagged. Returns an empty string
// when no drift is detected.
//
// hasDriver is taken as a parameter rather than re-probed via
// HasMdsmithMergeDriver so Check does not pay an extra `git config`
// subprocess per linted file: the caller has already computed it.
//
// An empty `merge=mdsmith` assignment list with the driver registered
// and discovered files present is treated as drift: the merge driver
// will not run for any file, defeating the registration. A non-ENOENT
// read error is surfaced as drift too rather than silently passing,
// so permission/IO failures cannot mask real misconfiguration.
func (r *Rule) mergeDriverDrift(repoRoot string, hasDriver bool, getDiscovered func() []string) string {
	if !hasDriver {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitattributes"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Sprintf(
			"cannot verify merge-driver assignments because .gitattributes could not be read: %v",
			err,
		)
	}
	discovered := getDiscovered()
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
func (r *Rule) preMergeCommitHookDrift(repoRoot string, getDiscovered func() []string) string {
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
	discovered := getDiscovered()
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

// Fix implements rule.FixableRule. It regenerates .gitattributes to
// match the discovered file list when the merge driver is registered.
// The pre-merge-commit hook is not auto-fixed because it is an
// executable script and modifying executable files during automated
// fixes could be surprising or unsafe. Users must run
// `mdsmith pre-merge-commit install` manually to update the hook.
//
// The fix only runs when f.FS != nil (a real file, not stdin) and
// when the repository has opted into the merge driver via
// `git config merge.mdsmith.driver`. If neither condition holds, the
// original file content is returned unchanged.
//
// Fix short-circuits via a FilesMatch check when .gitattributes is
// already in sync, so linting many files in the same repo does not
// trigger redundant rewrites. Subsequent calls may still do real
// work in two cases: drift has reappeared (e.g. an external tool or
// a different MaxInputBytes-driven discovery changed the expected
// set), or a previous staging attempt failed and needs retrying.
func (r *Rule) Fix(f *lint.File) []byte {
	// Skip stdin and other in-memory inputs (same logic as Check).
	if f.FS == nil {
		return f.Source
	}

	repoRoot, err := r.resolveRepoRoot(filepath.Dir(f.Path))
	if err != nil {
		return f.Source
	}

	// Only fix when the merge driver is registered. If the driver
	// isn't set up, there's no .gitattributes to repair.
	if !githooks.HasMdsmithMergeDriver(repoRoot) {
		return f.Source
	}

	discovered := r.getDiscovered(repoRoot, f.MaxInputBytes)
	attrPath := filepath.Join(repoRoot, ".gitattributes")

	// When .gitattributes is in sync, skip the rewrite. If a previous
	// run failed to stage, retry the staging step now so the pending
	// error is given a chance to clear without forcing a redundant
	// write. FilesMatch is the only short-circuit we need: there is no
	// per-process "already wrote" guard because every successful write
	// must flow into the staging path below — without that, a write
	// triggered by reappearing drift would update the working tree but
	// leave the index pointing at the old content.
	data, err := os.ReadFile(attrPath)
	if err == nil {
		installed := githooks.ExtractGitattributesFiles(string(data))
		if githooks.FilesMatch(installed, discovered) {
			if stagingError(repoRoot) != nil {
				stage(repoRoot)
			}
			return f.Source
		}
	}

	// Write the corrected .gitattributes. Any successful write flows
	// through the staging path so the index always reflects the
	// updated working-tree content; a transient write failure simply
	// leaves the tree unchanged so the next Fix call can retry.
	if err := githooks.WriteGitattributes(attrPath, discovered); err != nil {
		return f.Source
	}

	// Stage the regenerated .gitattributes so the pre-merge-commit
	// hook flow includes it in the merge commit alongside the
	// markdown files mdsmith fix touched. The error is recorded in
	// stagingErrors so Check can keep emitting a diagnostic until a
	// later Fix call's staging attempt succeeds; without that, a
	// transient `git add` failure (e.g. index.lock contention)
	// would silently leave the working tree fixed but the merge
	// commit missing the .gitattributes update.
	stage(repoRoot)

	// Return original file content unchanged (the fix is in .gitattributes,
	// not in the markdown file being linted)
	return f.Source
}

// stage attempts to stage .gitattributes and records the outcome in
// stagingErrors so Check can surface a persistent failure.
func stage(repoRoot string) {
	err := githooks.StageGitattributes(repoRoot)
	stagingMu.Lock()
	defer stagingMu.Unlock()
	if err != nil {
		stagingErrors[repoRoot] = err
		return
	}
	delete(stagingErrors, repoRoot)
}

// stagingError returns the most recent unsuccessful staging attempt
// for repoRoot, or nil if the last attempt succeeded (or there has
// been none).
func stagingError(repoRoot string) error {
	stagingMu.Lock()
	defer stagingMu.Unlock()
	return stagingErrors[repoRoot]
}

// resolveRepoRoot wraps githooks.GitRepoRoot with a per-directory
// cache so the per-file diagnostic flow does not respawn
// `git rev-parse --show-toplevel` for every linted file in the same
// repo. Failures (the linted file is not inside a git repo) are
// cached too so a directory tree without a `.git` ancestor is also
// only probed once.
func (r *Rule) resolveRepoRoot(dir string) (string, error) {
	repoRootMu.Lock()
	defer repoRootMu.Unlock()
	if entry, ok := repoRootCache[dir]; ok {
		return entry.root, entry.err
	}
	root, err := githooks.GitRepoRoot(dir)
	repoRootCache[dir] = repoRootEntry{root: root, err: err}
	return root, err
}

// getDiscovered returns the discovered files for a repo, using a cached
// result if available. Discovery is expensive (full repo walk + file
// reads), so caching it significantly improves performance when checking
// many files in the same repo. The cache key includes maxBytes because
// DiscoverFiles passes that limit to ReadFileLimited when scanning each
// file: a different limit can change which files qualify as
// directive-bearing, so reusing a slice computed under a different
// budget would return an incorrect list.
func (r *Rule) getDiscovered(repoRoot string, maxBytes int64) []string {
	discoveredMu.Lock()
	defer discoveredMu.Unlock()

	cacheKey := fmt.Sprintf("%s:%d", repoRoot, maxBytes)
	if files, ok := discoveredCache[cacheKey]; ok {
		return files
	}

	files := githooks.DiscoverFiles(repoRoot, maxBytes)
	discoveredCache[cacheKey] = files
	return files
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
	_ rule.FixableRule  = (*Rule)(nil)
)
