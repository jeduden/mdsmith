package fix

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alwaysFiringRule emits one diagnostic per line in the file. It is
// non-fixable, so any diagnostic the engine doesn't filter survives
// into the post-fix result.
type alwaysFiringRule struct {
	id   string
	name string
}

func (r *alwaysFiringRule) ID() string       { return r.id }
func (r *alwaysFiringRule) Name() string     { return r.name }
func (r *alwaysFiringRule) Category() string { return "test" }

func (r *alwaysFiringRule) Check(f *lint.File) []lint.Diagnostic {
	diags := make([]lint.Diagnostic, 0, len(f.Lines))
	for i := range f.Lines {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     i + 1,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  "always",
		})
	}
	return diags
}

// TestFix_SuppressesDiagnosticsInsideCatalogBody mirrors
// engine.TestLintOnce_CatalogHost on the fix path: diagnostics whose
// lines fall inside a <?catalog?> generated body must not surface
// from the host's perspective. Without this, `mdsmith fix` and
// `mdsmith check` disagree on the same source bytes — check filters
// the body, fix doesn't — which is what surfaced as a CI-only failure
// in the merge queue when the pre-merge-commit hook ran fix on a
// merge containing a long catalog summary.
func TestFix_SuppressesDiagnosticsInsideCatalogBody(t *testing.T) {
	dir := t.TempDir()

	// Lines:
	// 1: # Catalog Host
	// 2: (empty)
	// 3: <?catalog
	// 4: glob: "*.md"
	// 5: row: "- {filename}"
	// 6: ?>
	// 7: - body-line.md         <-- generated-body content (line 7)
	// 8: <?/catalog?>
	host := "# Catalog Host\n\n" +
		"<?catalog\nglob: \"*.md\"\nrow: \"- {filename}\"\n?>\n" +
		"- body-line.md\n" +
		"<?/catalog?>\n"
	hostPath := filepath.Join(dir, "host.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(host), 0o644))

	const ruleName = "always-fires"
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			ruleName: {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&alwaysFiringRule{id: "MDS999", name: ruleName}},
	}

	result := fixer.Fix([]string{hostPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	// Line 7 is inside the generated catalog body. A diagnostic on
	// that line must be filtered out before reaching the result.
	for _, d := range result.Diagnostics {
		if d.RuleName == ruleName && d.Line == 7 {
			t.Errorf("fix surfaced %s diagnostic from inside <?catalog?> body (line 7): %s",
				ruleName, d.Message)
		}
	}

	// Pre-fix Failures count must also exclude the generated-body
	// line. The host has 9 line entries (8 source lines plus the
	// trailing-newline split) but line 7 is inside the generated
	// body, so fewer than 9 should remain after generated-body
	// filtering. The strict-less assertion stays robust to unrelated
	// trailing-line counting tweaks.
	assert.Less(t, result.Failures, 9,
		"pre-fix Failures should exclude the generated-body line, got %d (no filtering applied)",
		result.Failures)
}

// gitignoreProbeRule records, on every Check call, whether the file
// has a non-nil GitignoreFunc set. Rules in the wild (catalog uses
// f.GetGitignore() in resolveGitignore) silently lose gitignore
// filtering when the matcher is nil — so a missing GitignoreFunc on
// the Fixer's lint.File causes catalog output to disagree between
// `mdsmith fix` and `mdsmith check` on the same source bytes.
type gitignoreProbeRule struct {
	id         string
	name       string
	hasMatcher []bool
}

func (r *gitignoreProbeRule) ID() string       { return r.id }
func (r *gitignoreProbeRule) Name() string     { return r.name }
func (r *gitignoreProbeRule) Category() string { return "test" }
func (r *gitignoreProbeRule) Check(f *lint.File) []lint.Diagnostic {
	r.hasMatcher = append(r.hasMatcher, f.GetGitignore() != nil)
	return nil
}

// TestFix_FilesHaveGitignoreFunc verifies that the Fixer wires
// GitignoreFunc onto its *lint.File the same way the Runner does, so
// rules that consult f.GetGitignore() (catalog) behave identically
// during fix and check. Without this, a catalog directive whose glob
// matches gitignored files would include those files in the fix-time
// regenerated catalog body but exclude them at check time.
func TestFix_FilesHaveGitignoreFunc(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# Doc\n"), 0o644))

	const ruleName = "gitignore-probe"
	probe := &gitignoreProbeRule{id: "MDS997", name: ruleName}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			ruleName: {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{probe},
	}

	result := fixer.Fix([]string{mdPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	// Pre-fix and post-fix Check calls; both must see a usable
	// gitignore matcher (parity with engine.Runner's per-file setup).
	require.Len(t, probe.hasMatcher, 2,
		"expected one Check call pre-fix and one post-fix, got %d", len(probe.hasMatcher))
	for i, ok := range probe.hasMatcher {
		assert.True(t, ok,
			"call %d: f.GetGitignore() returned nil — Fixer did not wire GitignoreFunc", i)
	}
}

// TestFixer_CachedGitignore_DistinctKeys directly exercises the
// cache contract: distinct inputs must yield distinct matchers, and
// a repeat input must hit the cache. The previous implementation
// canonicalized via filepath.Abs and silently swallowed Abs's error
// return, which would collapse every relative path to the empty
// string ("") cache key on Abs failure (e.g., unreadable cwd) and
// share one matcher across unrelated directories.
func TestFixer_CachedGitignore_DistinctKeys(t *testing.T) {
	fixer := &Fixer{}

	a1 := fixer.cachedGitignore("/tmp/aaa")
	b := fixer.cachedGitignore("/tmp/bbb")
	a2 := fixer.cachedGitignore("/tmp/aaa")
	empty1 := fixer.cachedGitignore("")
	empty2 := fixer.cachedGitignore("")

	require.NotNil(t, a1)
	require.NotNil(t, b)
	require.NotNil(t, empty1)

	assert.NotSame(t, a1, b,
		"different directories must produce different matchers; the previous "+
			"Abs-no-fallback impl shared one cache entry across unrelated dirs "+
			"on the Abs-error path")
	assert.Same(t, a1, a2,
		"repeated input must hit the cache and return the same matcher pointer")
	assert.Same(t, empty1, empty2,
		"empty-string input is its own cache entry, not aliased with /tmp/aaa")
	assert.NotSame(t, a1, empty1,
		"empty-string input must not collide with /tmp/aaa")
}

// TestFix_FilesHaveGitignoreFuncWithRootDir covers the prepareFile
// branch where Fixer.RootDir is set (gitignore matcher is anchored at
// the root rather than the file's directory). Without a test that
// configures RootDir, the branch flipping gitignoreDir = f.RootDir is
// never executed.
func TestFix_FilesHaveGitignoreFuncWithRootDir(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# Doc\n"), 0o644))

	const ruleName = "gitignore-probe-root"
	probe := &gitignoreProbeRule{id: "MDS995", name: ruleName}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			ruleName: {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config:  cfg,
		Rules:   []rule.Rule{probe},
		RootDir: dir,
	}

	result := fixer.Fix([]string{mdPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)
	require.Len(t, probe.hasMatcher, 2,
		"expected pre-fix and post-fix Check calls, got %d", len(probe.hasMatcher))
	for i, ok := range probe.hasMatcher {
		assert.True(t, ok,
			"call %d: f.GetGitignore() returned nil under RootDir mode", i)
	}
}

// fixPassProbeRule is a fixable rule that records, on each Check
// invocation inside applyFixPasses, whether the parsedFile carries
// the per-file context (MaxInputBytes / StripFrontMatter /
// GitignoreFunc / GeneratedRanges) that the engine.Runner sets. Its
// Fix returns the source unchanged so applyFixPasses stabilizes
// after one iteration.
type fixPassProbeRule struct {
	id       string
	name     string
	maxBytes []int64
	stripFM  []bool
	hasGI    []bool
	hasRange []bool
}

func (r *fixPassProbeRule) ID() string       { return r.id }
func (r *fixPassProbeRule) Name() string     { return r.name }
func (r *fixPassProbeRule) Category() string { return "test" }
func (r *fixPassProbeRule) Check(f *lint.File) []lint.Diagnostic {
	r.maxBytes = append(r.maxBytes, f.MaxInputBytes)
	r.stripFM = append(r.stripFM, f.StripFrontMatter)
	r.hasGI = append(r.hasGI, f.GetGitignore() != nil)
	r.hasRange = append(r.hasRange, len(f.GeneratedRanges) > 0)
	// Return one diagnostic on the first call so applyFixPasses also
	// invokes Fix; subsequent calls return nil so the loop stabilizes.
	if len(r.maxBytes) == 1 {
		return []lint.Diagnostic{{
			File: f.Path, Line: 1, Column: 1,
			RuleID: r.id, RuleName: r.name,
			Severity: lint.Warning, Message: "trigger fix",
		}}
	}
	return nil
}
func (r *fixPassProbeRule) Fix(f *lint.File) []byte { return f.Source }

var _ rule.FixableRule = (*fixPassProbeRule)(nil)

// TestFix_FixPassesHydrateLintFile verifies that the parsedFile used
// inside applyFixPasses carries the same per-file context that the
// pre-fix and post-fix CheckRules calls already use. Without this,
// fixable rules that consult these fields during their own Check or
// Fix (notably catalog: GetGitignore for glob filtering, include:
// MaxInputBytes for secondary reads) silently produce different
// post-fix bytes than `mdsmith check` would have validated against.
func TestFix_FixPassesHydrateLintFile(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	host := "---\ntitle: t\n---\n# Doc\n\n" +
		"<?catalog\nglob: \"*.md\"\nrow: \"- {filename}\"\n?>\n" +
		"- doc.md\n" +
		"<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdPath, []byte(host), 0o644))

	const ruleName = "fix-pass-probe"
	const wantMaxBytes int64 = 8192
	probe := &fixPassProbeRule{id: "MDS996", name: ruleName}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			ruleName: {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config:           cfg,
		Rules:            []rule.Rule{probe},
		StripFrontMatter: true,
		MaxInputBytes:    wantMaxBytes,
	}

	result := fixer.Fix([]string{mdPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	// applyFixPasses calls Check at least once on parsedFile; that
	// parsedFile must carry the per-file context.
	require.NotEmpty(t, probe.maxBytes,
		"fixable rule was never invoked inside applyFixPasses")
	for i, got := range probe.maxBytes {
		assert.Equal(t, wantMaxBytes, got,
			"fix-pass call %d: MaxInputBytes not propagated to parsedFile", i)
	}
	for i, got := range probe.stripFM {
		assert.True(t, got,
			"fix-pass call %d: StripFrontMatter not propagated to parsedFile", i)
	}
	for i, got := range probe.hasGI {
		assert.True(t, got,
			"fix-pass call %d: GitignoreFunc not propagated to parsedFile", i)
	}
	for i, got := range probe.hasRange {
		assert.True(t, got,
			"fix-pass call %d: GeneratedRanges not populated on parsedFile (catalog body unprotected)", i)
	}
}

// fieldRecordingRule captures f.MaxInputBytes and f.StripFrontMatter
// on every Check invocation. Rules in the wild (catalog, include,
// duplicatedcontent, requiredstructure, crossfilereferenceintegrity)
// consult these fields when reading secondary files; if the Fixer's
// post-fix lint pass receives a *lint.File where these are zero, the
// post-fix CheckRules call diverges from the pre-fix call and from
// the Runner.
type fieldRecordingRule struct {
	id       string
	name     string
	maxBytes []int64
	stripFM  []bool
}

func (r *fieldRecordingRule) ID() string       { return r.id }
func (r *fieldRecordingRule) Name() string     { return r.name }
func (r *fieldRecordingRule) Category() string { return "test" }
func (r *fieldRecordingRule) Check(f *lint.File) []lint.Diagnostic {
	r.maxBytes = append(r.maxBytes, f.MaxInputBytes)
	r.stripFM = append(r.stripFM, f.StripFrontMatter)
	return nil
}

// TestFix_FinalFileInheritsParseTimeFields verifies that the post-fix
// CheckRules call receives a *lint.File whose MaxInputBytes and
// StripFrontMatter mirror the pre-fix file's values. The Runner
// already establishes these on its single File before linting; the
// Fixer parses a fresh finalFile after applyFixPasses and must
// propagate the same fields, otherwise rules that consult them (e.g.
// catalog/include reading secondary files, duplicatedcontent computing
// cross-file coordinates) silently diverge between fix and check on
// the same source bytes.
func TestFix_FinalFileInheritsParseTimeFields(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("---\ntitle: t\n---\n# Doc\n"), 0o644))

	const ruleName = "field-recorder"
	const wantMaxBytes int64 = 4096
	rec := &fieldRecordingRule{id: "MDS998", name: ruleName}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			ruleName: {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config:           cfg,
		Rules:            []rule.Rule{rec},
		StripFrontMatter: true,
		MaxInputBytes:    wantMaxBytes,
	}

	result := fixer.Fix([]string{mdPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	// Two CheckRules calls per file: one pre-fix on lf, one post-fix
	// on finalFile. Both must see the configured values.
	require.Len(t, rec.maxBytes, 2,
		"expected one Check call pre-fix and one post-fix, got %d", len(rec.maxBytes))
	for i, got := range rec.maxBytes {
		assert.Equal(t, wantMaxBytes, got,
			"call %d: MaxInputBytes not propagated to lint.File", i)
	}
	for i, got := range rec.stripFM {
		assert.True(t, got,
			"call %d: StripFrontMatter not propagated to lint.File", i)
	}
}
