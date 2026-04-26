package engine

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

// configurableMockRule is a test rule whose behavior is parameterized by a
// boolean "report" setting. When enabled with report=true, it emits a
// diagnostic; with report=false (or unset), it stays silent.
type configurableMockRule struct {
	id     string
	name   string
	report bool
}

func (r *configurableMockRule) ID() string       { return r.id }
func (r *configurableMockRule) Name() string     { return r.name }
func (r *configurableMockRule) Category() string { return "test" }
func (r *configurableMockRule) Check(f *lint.File) []lint.Diagnostic {
	if !r.report {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.id,
		RuleName: r.name,
		Severity: lint.Warning,
		Message:  "configurable mock fired",
	}}
}

func (r *configurableMockRule) ApplySettings(s map[string]any) error {
	if v, ok := s["report"].(bool); ok {
		r.report = v
	}
	return nil
}

func (r *configurableMockRule) DefaultSettings() map[string]any {
	return map[string]any{"report": false}
}

// TestRunner_KindAssignmentAppliesSettings verifies that a kind-assignment
// glob causes the kind's rule settings to be applied to matching files.
func TestRunner_KindAssignmentAppliesSettings(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "plan.md")
	require.NoError(t, os.WriteFile(planFile, []byte("# Plan\n"), 0o644))
	regFile := filepath.Join(dir, "regular.md")
	require.NoError(t, os.WriteFile(regFile, []byte("# Regular\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"configurable-mock": {Enabled: true},
		},
		Kinds: map[string]config.Kind{
			"plan": {Rules: map[string]config.RuleCfg{
				"configurable-mock": {Enabled: true, Settings: map[string]any{"report": true}},
			}},
		},
		KindAssignment: []config.KindAssignment{
			{Files: []string{"plan.md"}, Kinds: []string{"plan"}},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&configurableMockRule{id: "MDS997", name: "configurable-mock"}},
	}

	res := runner.Run([]string{planFile, regFile})
	require.Empty(t, res.Errors, "unexpected errors: %v", res.Errors)
	require.Len(t, res.Diagnostics, 1, "expected 1 diagnostic from plan.md only")
	assert.Equal(t, planFile, res.Diagnostics[0].File)
}

// TestRunner_FrontMatterKindsApplySettings verifies that a file's
// front-matter `kinds:` list applies the named kind's rule settings.
func TestRunner_FrontMatterKindsApplySettings(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.md")
	body := "---\nkinds: [plan]\n---\n# Doc\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(body), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"configurable-mock": {Enabled: true},
		},
		Kinds: map[string]config.Kind{
			"plan": {Rules: map[string]config.RuleCfg{
				"configurable-mock": {Enabled: true, Settings: map[string]any{"report": true}},
			}},
		},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&configurableMockRule{id: "MDS997", name: "configurable-mock"}},
		StripFrontMatter: true,
	}

	res := runner.Run([]string{mdFile})
	require.Empty(t, res.Errors)
	require.Len(t, res.Diagnostics, 1,
		"expected diagnostic since front-matter kind enabled report=true")
}

// TestRunner_UndeclaredKindFromFrontMatterErrors verifies that referencing
// an undeclared kind in a file's front matter surfaces as a config error
// in the run result.
func TestRunner_UndeclaredKindFromFrontMatterErrors(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.md")
	body := "---\nkinds: [missing]\n---\n# Doc\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(body), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"configurable-mock": {Enabled: true},
		},
		Kinds: map[string]config.Kind{
			"plan": {},
		},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&configurableMockRule{id: "MDS997", name: "configurable-mock"}},
		StripFrontMatter: true,
	}

	res := runner.Run([]string{mdFile})
	require.NotEmpty(t, res.Errors, "expected a config error for undeclared kind")
	found := false
	for _, e := range res.Errors {
		if e != nil && contains(e.Error(), "missing") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected error to mention undeclared kind name")
}

// TestRunner_LaterKindBlockReplacesEarlier verifies that two kinds bound
// to a file that both configure the same rule resolve by block
// replacement: the later kind's block wins.
func TestRunner_LaterKindBlockReplacesEarlier(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.md")
	body := "---\nkinds: [a, b]\n---\n# Doc\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(body), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"configurable-mock": {Enabled: true},
		},
		Kinds: map[string]config.Kind{
			"a": {Rules: map[string]config.RuleCfg{
				"configurable-mock": {Enabled: true, Settings: map[string]any{"report": true}},
			}},
			"b": {Rules: map[string]config.RuleCfg{
				"configurable-mock": {Enabled: true, Settings: map[string]any{"report": false}},
			}},
		},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&configurableMockRule{id: "MDS997", name: "configurable-mock"}},
		StripFrontMatter: true,
	}

	res := runner.Run([]string{mdFile})
	require.Empty(t, res.Errors)
	assert.Empty(t, res.Diagnostics, "later kind b (report=false) should replace earlier kind a")
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
