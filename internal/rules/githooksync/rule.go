// Package githooksync implements MDS048, the git-hook-sync rule. It
// reports when the .gitattributes managed block or the
// pre-merge-commit hook drifts from the canonical content derived
// from the project's .mdsmith.yml ignore patterns.
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
// in sync with the canonical content computed from the project's
// .mdsmith.yml ignore patterns.
//
// The rule is runnable in its zero value: it has no required runtime
// settings, so users can opt in via the bool form `git-hook-sync:
// true`. ApplySettings is still implemented to validate unknown keys
// when the user provides a mapping, but execution does not depend on
// it being called.
type Rule struct{}

// stagingErrors records repos where Fix wrote .gitattributes but the
// follow-up `git add -- .gitattributes` failed (e.g. index.lock
// contention). The on-disk fix already happened, so a plain drift
// re-check would see the file as in sync and stop emitting
// diagnostics — silently leaving the staged tree out of sync with
// the working tree. Surfacing the failure through Check makes it
// retryable: subsequent Fix calls re-run the staging step until it
// succeeds, at which point the entry is cleared.
//
// repoRootCache memoises the result of GitRepoRoot(dir) so per-file
// Check/Fix calls do not respawn `git rev-parse --show-toplevel` for
// every file in the same directory. Entries with a non-nil error are
// also cached so non-repo directories are remembered too. The cache
// is keyed by the directory passed to resolveRepoRoot, not the
// resolved root, so repeated lookups for the same directory reuse
// one git invocation; different subdirectories under the same repo
// may still invoke git separately.
//
// driftCache memoises the per-repo drift messages computed by Check
// (the merge-driver and pre-merge-commit hook sources, excluding the
// dynamic staging-error part). Without it, Check would respawn
// `git config`, re-read the hook script, and re-parse `.mdsmith.yml`
// once per linted file. Fix() invalidates the entry so the next
// Check observes the post-fix state.
var (
	stagingMu     sync.Mutex
	stagingErrors = make(map[string]error)
	repoRootMu    sync.Mutex
	repoRootCache = make(map[string]repoRootEntry)
	driftMu       sync.Mutex
	driftCache    = make(map[string]driftResult)
)

type repoRootEntry struct {
	root string
	err  error
}

// driftResult is the cached output of Check's repo-level drift
// inspection minus the staging-error message, which is checked on
// every call because it can change between Check and Fix calls.
type driftResult struct {
	parts []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS048" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "git-hook-sync" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

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
	repoRoot, err := r.resolveRepoRoot(filepath.Dir(f.Path))
	if err != nil {
		return nil
	}

	// Look up the cached drift inspection first. Without this, every
	// linted markdown file would respawn `git config`, re-read the
	// hook script, and re-parse `.mdsmith.yml`; over a large repo
	// that becomes O(files) git subprocesses + YAML parses for a
	// single repo-level diagnostic. Fix() invalidates the cache
	// entry so the post-fix state is observed.
	parts := r.driftParts(repoRoot)
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

	// Anchor the diagnostic to the .gitattributes path (the
	// repo-level artifact this rule checks) rather than the
	// markdown file the engine happened to be linting. Drift is a
	// repo-level concern, and pointing every duplicate report at
	// the same artifact path lets downstream tooling collapse them
	// by (file, line, ruleID) and makes the source of the warning
	// explicit in the diagnostic output.
	diagPath := filepath.Join(repoRoot, ".gitattributes")
	return []lint.Diagnostic{{
		File:     diagPath,
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
// hook without parsing its contents.
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

// driftParts returns the cached drift messages for repoRoot,
// computing them on a cache miss. The result excludes the dynamic
// staging-error part (Check appends that on every call so a
// staging failure recorded after the first inspection is still
// surfaced). Fix() invalidates the cache entry so the post-fix
// state is observed; without invalidation a successful Fix would
// still report stale drift.
func (r *Rule) driftParts(repoRoot string) []string {
	driftMu.Lock()
	if cached, ok := driftCache[repoRoot]; ok {
		driftMu.Unlock()
		return cached.parts
	}
	driftMu.Unlock()

	hasDriver := githooks.HasMdsmithMergeDriver(repoRoot)
	hookState := peekHookSource(repoRoot)
	if !hasDriver && hookState != hookSourceManaged && hookState != hookSourceUnreadable {
		driftMu.Lock()
		driftCache[repoRoot] = driftResult{}
		driftMu.Unlock()
		return nil
	}

	expectedGlobs := githooks.LoadGlobs(repoRoot)
	var parts []string
	if msg := r.mergeDriverDrift(repoRoot, hasDriver, expectedGlobs); msg != "" {
		parts = append(parts, msg)
	}
	if msg := r.preMergeCommitHookDrift(repoRoot); msg != "" {
		parts = append(parts, msg)
	}

	driftMu.Lock()
	driftCache[repoRoot] = driftResult{parts: parts}
	driftMu.Unlock()
	return parts
}

// mergeDriverDrift returns a human-readable description of any drift
// between the .gitattributes managed block and the canonical block
// derived from .mdsmith.yml. The check only runs when
// `merge.mdsmith.driver` is registered, so repos that have not opted
// in are not flagged. Returns an empty string when no drift is
// detected.
//
// hasDriver is taken as a parameter rather than re-probed via
// HasMdsmithMergeDriver so Check does not pay an extra `git config`
// subprocess per linted file: the caller has already computed it.
//
// A non-ENOENT read error is surfaced as drift rather than silently
// passing, so permission/IO failures cannot mask real misconfiguration.
func (r *Rule) mergeDriverDrift(repoRoot string, hasDriver bool, expected githooks.Globs) string {
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
	installed, ok := githooks.ExtractGlobs(string(data))
	if !ok {
		return fmt.Sprintf(
			"merge.mdsmith.driver is registered but .gitattributes has no managed block "+
				"(should contain include patterns: %s; exclude patterns: %s)",
			strings.Join(expected.Include, ", "),
			describeGlobs(expected.Exclude),
		)
	}
	if githooks.GlobsEqual(installed, expected) {
		return ""
	}
	return fmt.Sprintf(
		".gitattributes managed block is out of sync "+
			"(has include: %s, exclude: %s; should have include: %s, exclude: %s)",
		describeGlobs(installed.Include),
		describeGlobs(installed.Exclude),
		describeGlobs(expected.Include),
		describeGlobs(expected.Exclude),
	)
}

// describeGlobs returns a printable representation of patterns so
// "(none)" is shown for an empty list rather than a blank field.
func describeGlobs(patterns []string) string {
	if len(patterns) == 0 {
		return "(none)"
	}
	return strings.Join(patterns, ", ")
}

// preMergeCommitHookDrift returns a human-readable description of any
// drift between the installed pre-merge-commit hook and the canonical
// hook content. Returns an empty string if no hook is installed, the
// hook is not mdsmith-managed, or the content matches. A non-ENOENT
// read error is surfaced rather than silently passing so permission
// or IO failures cannot mask real drift.
func (r *Rule) preMergeCommitHookDrift(repoRoot string) string {
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
	// The canonical hook content depends on the absolute path of the
	// mdsmith binary that originally installed it. Comparing only the
	// portions that are independent of that path (the marker plus the
	// glob-based fix invocation pattern) keeps drift detection
	// hermetic across machines while still catching missing or
	// outdated hook content.
	if githooks.HookMatchesCanonical(hook) {
		return ""
	}
	return "pre-merge-commit hook is out of sync with the glob-based template " +
		"(re-run `mdsmith pre-merge-commit install` to update it)"
}

// Fix implements rule.FixableRule. It regenerates the .gitattributes
// managed block from the canonical glob set when the merge driver is
// registered. The pre-merge-commit hook is not auto-fixed because it
// is an executable script and modifying executable files during
// automated fixes could be surprising or unsafe. Users must run
// `mdsmith pre-merge-commit install` manually to update the hook.
//
// The fix only runs when f.FS != nil (a real file, not stdin) and
// when the repository has opted into the merge driver via
// `git config merge.mdsmith.driver`. If neither condition holds, the
// original file content is returned unchanged.
//
// Fix short-circuits via a GlobsEqual check when .gitattributes is
// already in sync, so linting many files in the same repo does not
// trigger redundant rewrites. Subsequent calls may still do real
// work in two cases: drift has reappeared (e.g. an external tool
// changed the managed block), or a previous staging attempt failed
// and needs retrying.
func (r *Rule) Fix(f *lint.File) []byte {
	// Skip stdin and other in-memory inputs (same logic as Check).
	if f.FS == nil {
		return f.Source
	}

	repoRoot, err := r.resolveRepoRoot(filepath.Dir(f.Path))
	if err != nil {
		return f.Source
	}

	// Whatever Fix decides, the cached drift result for this repo
	// is now stale (the on-disk state may have changed). Drop the
	// entry so the next Check re-reads the repo. Doing this even
	// when Fix short-circuits keeps the cache coherent if the
	// driver is registered between calls or an external tool
	// rewrites .gitattributes.
	defer func() {
		driftMu.Lock()
		delete(driftCache, repoRoot)
		driftMu.Unlock()
	}()

	// Only fix when the merge driver is registered. If the driver
	// isn't set up, there's no .gitattributes to repair.
	if !githooks.HasMdsmithMergeDriver(repoRoot) {
		return f.Source
	}

	expected := githooks.LoadGlobs(repoRoot)
	attrPath := filepath.Join(repoRoot, ".gitattributes")

	// When .gitattributes is already in sync, skip the rewrite. If a
	// previous run failed to stage, retry the staging step now so the
	// pending error is given a chance to clear without forcing a
	// redundant write.
	data, err := os.ReadFile(attrPath)
	if err == nil {
		installed, ok := githooks.ExtractGlobs(string(data))
		if ok && githooks.GlobsEqual(installed, expected) {
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
	if err := githooks.WriteGitattributes(attrPath, expected); err != nil {
		return f.Source
	}

	// Stage the regenerated .gitattributes so the pre-merge-commit
	// hook flow includes it in the merge commit alongside the
	// markdown files mdsmith fix touched. The error is recorded in
	// stagingErrors so Check can keep emitting a diagnostic until a
	// later Fix call's staging attempt succeeds.
	stage(repoRoot)

	// Return original file content unchanged (the fix is in
	// .gitattributes, not in the markdown file being linted).
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
//
// The mutex is released across the (potentially slow) `git rev-parse`
// subprocess so concurrent Check/Fix calls on unrelated directories
// are not serialised behind a single git invocation. A second
// directory triggering the same lookup may run git in parallel; the
// double-check after re-acquiring the lock makes the first writer
// win without hurting correctness.
func (r *Rule) resolveRepoRoot(dir string) (string, error) {
	repoRootMu.Lock()
	if entry, ok := repoRootCache[dir]; ok {
		repoRootMu.Unlock()
		return entry.root, entry.err
	}
	repoRootMu.Unlock()

	root, err := githooks.GitRepoRoot(dir)

	repoRootMu.Lock()
	defer repoRootMu.Unlock()
	if entry, ok := repoRootCache[dir]; ok {
		// A concurrent caller populated the cache while we were
		// running `git rev-parse`; honour their result so callers
		// observe a single source of truth.
		return entry.root, entry.err
	}
	repoRootCache[dir] = repoRootEntry{root: root, err: err}
	return root, err
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
