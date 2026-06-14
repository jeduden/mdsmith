package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Production rule set for the regression guard.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// repoScopedMock is a rule that always emits a diagnostic anchored to a
// fixed cross-file path regardless of the linted host file, and implements
// rule.RepoScoped to signal this.
type repoScopedMock struct {
	id, name, crossFilePath string
}

func (r *repoScopedMock) ID() string       { return r.id }
func (r *repoScopedMock) Name() string     { return r.name }
func (r *repoScopedMock) Category() string { return "test" }
func (r *repoScopedMock) Check(_ *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{{
		File:     r.crossFilePath,
		Line:     1,
		Column:   1,
		RuleID:   r.id,
		RuleName: r.name,
		Severity: lint.Warning,
		Message:  "repo-level finding",
	}}
}
func (r *repoScopedMock) RepoScopedDiagnostics() bool { return true }

// TestAnyRepoScopedEnabled_NoRepoScopedRules verifies that a Runner with
// only non-RepoScoped rules reports false so the DedupeDiagnostics call
// is skipped.
func TestAnyRepoScopedEnabled_NoRepoScopedRules(t *testing.T) {
	r := &Runner{
		Config: &config.Config{
			Rules: map[string]config.RuleCfg{
				"mock-rule": {Enabled: true},
			},
		},
		Rules: []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
	}
	assert.False(t, r.anyRepoScopedEnabled(),
		"runner with no RepoScoped rule must return false")
}

// TestAnyRepoScopedEnabled_TrueWhenEnabled verifies that a Runner with an
// enabled RepoScoped rule reports true so DedupeDiagnostics is called.
func TestAnyRepoScopedEnabled_TrueWhenEnabled(t *testing.T) {
	r := &Runner{
		Config: &config.Config{
			Rules: map[string]config.RuleCfg{
				"repo-mock": {Enabled: true},
			},
		},
		Rules: []rule.Rule{
			&repoScopedMock{id: "TST001", name: "repo-mock", crossFilePath: "/repo/.gitattributes"},
		},
	}
	assert.True(t, r.anyRepoScopedEnabled(),
		"runner with enabled RepoScoped rule must return true")
}

// TestAnyRepoScopedEnabled_FalseWhenDisabled verifies that a Runner with a
// disabled RepoScoped rule still returns false (no dedupe needed).
func TestAnyRepoScopedEnabled_FalseWhenDisabled(t *testing.T) {
	r := &Runner{
		Config: &config.Config{
			Rules: map[string]config.RuleCfg{
				"repo-mock": {Enabled: false},
			},
		},
		Rules: []rule.Rule{
			&repoScopedMock{id: "TST001", name: "repo-mock", crossFilePath: "/repo/.gitattributes"},
		},
	}
	assert.False(t, r.anyRepoScopedEnabled(),
		"runner with disabled RepoScoped rule must return false")
}

// TestRunEquivalenceRepoScopedDedupe verifies that Run output is byte-identical
// whether DedupeDiagnostics is applied unconditionally or skipped (the skip
// path). With only non-RepoScoped rules enabled, no cross-file duplicate
// tuples can arise, so the conditional and unconditional paths must agree.
func TestRunEquivalenceRepoScopedDedupe(t *testing.T) {
	dir := t.TempDir()

	// Two-file corpus with headings so MDS004 (first-line-heading) passes.
	files := make([]string, 2)
	for i := range files {
		p := filepath.Join(dir, fmt.Sprintf("doc%d.md", i))
		content := fmt.Sprintf("# Document %d\n\nA sentence for the test.\n", i)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		files[i] = p
	}

	// Use the full default production rule set (no RepoScoped rules enabled
	// by default, so anyRepoScopedEnabled returns false → skip path).
	cfg := config.Defaults()
	runnerFn := func() *Runner {
		return &Runner{
			Config:            cfg,
			Rules:             rule.All(),
			RootDir:           dir,
			StripFrontMatter:  true,
			SkipSourceContext: true,
		}
	}

	// Run with the conditional skip path (current production behaviour).
	res := runnerFn().Run(files)
	require.Empty(t, res.Errors)

	// Applying DedupeDiagnostics to an already-clean output must be a no-op:
	// the two slices must be equal, confirming the skip is safe.
	deduped := lint.DedupeDiagnostics(res.Diagnostics)
	assert.Equal(t, res.Diagnostics, deduped,
		"DedupeDiagnostics applied to non-repo-scoped output must be a no-op; "+
			"skip path produced duplicates when it should not have")

	// Sanity: running with an enabled RepoScoped rule still produces
	// correct (deduped) output by taking the non-skip path.
	crossPath := filepath.Join(dir, "fake-artifact")
	cfgRS := &config.Config{
		Rules: map[string]config.RuleCfg{
			"repo-mock": {Enabled: true},
		},
	}
	rsMock := &repoScopedMock{id: "TST001", name: "repo-mock", crossFilePath: crossPath}
	resRS := (&Runner{
		Config:            cfgRS,
		Rules:             []rule.Rule{rsMock},
		RootDir:           dir,
		SkipSourceContext: true,
	}).Run(files)
	require.Empty(t, resRS.Errors)
	// Both files emitted the same diagnostic; dedupe must have collapsed them.
	assert.Len(t, resRS.Diagnostics, 1,
		"RepoScoped rule emitting the same tuple for each linted file must be deduped to one entry")
}

// TestNonRepoScopedRulesNoCrossFileDuplicate is a regression guard. It runs
// the full production rule set against a multi-file corpus, linting each
// file independently and collecting per-file diagnostics. Any diagnostic
// anchored to a file other than the linted host file must come from a rule
// that implements rule.RepoScoped — otherwise a future unmarked rule could
// silently leak duplicates when DedupeDiagnostics is skipped.
func TestNonRepoScopedRulesNoCrossFileDuplicate(t *testing.T) {
	dir := t.TempDir()

	// Build a 3-file corpus. Files link to one another so cross-file
	// reference rules (MDS027) have something to inspect; the targets
	// exist so no link-broken diagnostics fire, keeping noise low.
	const n = 3
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("doc%d.md", i)
		next := fmt.Sprintf("doc%d.md", (i+1)%n)
		content := fmt.Sprintf(
			"# Document %d\n\nSee [the next doc](%s) for details.\n",
			i, next,
		)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	// Build a lookup from rule ID to rule instance so we can query
	// RepoScoped after the fact.
	allRules := rule.All()
	ruleByID := make(map[string]rule.Rule, len(allRules))
	for _, rl := range allRules {
		ruleByID[rl.ID()] = rl
	}

	cfg := config.Defaults()

	// Lint each file independently and check every diagnostic.
	for i := 0; i < n; i++ {
		host := filepath.Join(dir, fmt.Sprintf("doc%d.md", i))
		runner := &Runner{
			Config:            cfg,
			Rules:             allRules,
			RootDir:           dir,
			StripFrontMatter:  true,
			SkipSourceContext: true,
		}
		res := runner.Run([]string{host})
		require.Empty(t, res.Errors, "lint errors linting %s: %v", host, res.Errors)

		for _, d := range res.Diagnostics {
			if d.File == host {
				continue // anchored to linted file — always safe
			}
			// Cross-file anchor: the rule must declare itself RepoScoped.
			rl := ruleByID[d.RuleID]
			require.NotNil(t, rl,
				"diagnostic references unknown rule ID %s", d.RuleID)
			rs, ok := rl.(rule.RepoScoped)
			assert.True(t, ok && rs.RepoScopedDiagnostics(),
				"rule %s (%s) emits diagnostic for %q while linting %q "+
					"but does not implement rule.RepoScoped; "+
					"mark it RepoScoped or anchor its diagnostics to the host file",
				d.RuleID, d.RuleName, d.File, host)
		}
	}
}
