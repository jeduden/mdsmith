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

// configurableRule is a mock configurable rule whose "enabled" setting can
// be toggled by ApplySettings to verify kind-driven config.
type configurableRule struct {
	id      string
	name    string
	enabled bool
}

func (r *configurableRule) ID() string       { return r.id }
func (r *configurableRule) Name() string     { return r.name }
func (r *configurableRule) Category() string { return "test" }
func (r *configurableRule) Check(f *lint.File) []lint.Diagnostic {
	if !r.enabled {
		return nil
	}
	return []lint.Diagnostic{{
		File: f.Path, Line: 1, Column: 1,
		RuleID: r.id, RuleName: r.name, Severity: lint.Warning,
		Message: "triggered",
	}}
}
func (r *configurableRule) CloneRule() rule.Rule { return &configurableRule{id: r.id, name: r.name} }
func (r *configurableRule) ApplySettings(s map[string]any) error {
	if v, ok := s["enabled"].(bool); ok {
		r.enabled = v
	}
	return nil
}
func (r *configurableRule) DefaultSettings() map[string]any {
	return map[string]any{"enabled": true}
}

var _ rule.Configurable = (*configurableRule)(nil)

// TestKindAssignment_ConfiguresRuleSettings verifies that a kind's rule
// settings are applied via ApplySettings, enabling per-kind rule behavior
// beyond simple enable/disable.
func TestKindAssignment_ConfiguresRuleSettings(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "plan", "001_foo.md")
	otherFile := filepath.Join(dir, "other.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(planFile), 0o755))
	require.NoError(t, os.WriteFile(planFile, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(otherFile, []byte("# Hello\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			// Global settings disable the rule via its own "enabled" setting.
			"mock-configurable": {Enabled: true, Settings: map[string]any{"enabled": false}},
		},
		Kinds: map[string]config.KindBody{
			"plan": {Rules: map[string]config.RuleCfg{
				// Kind re-enables the rule via settings.
				"mock-configurable": {Enabled: true, Settings: map[string]any{"enabled": true}},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"**/plan/*.md"}, Kinds: []string{"plan"}},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&configurableRule{id: "MDS998", name: "mock-configurable"}},
	}

	result := runner.Run([]string{planFile, otherFile})
	require.Empty(t, result.Errors)
	require.Len(t, result.Diagnostics, 1, "kind settings should enable the rule for plan files only")
	assert.Equal(t, planFile, result.Diagnostics[0].File)
}

// TestKindAssignment_DisablesRule verifies that a kind assigned via
// kind-assignment disables a rule for matching files.
func TestKindAssignment_DisablesRule(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "plan", "001_foo.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(mdFile), 0o755))
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			"plan": {Rules: map[string]config.RuleCfg{
				"mock-rule": {Enabled: false},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"**/plan/*.md"}, Kinds: []string{"plan"}},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
	}

	result := runner.Run([]string{mdFile})
	require.Empty(t, result.Errors)
	assert.Empty(t, result.Diagnostics, "kind should have disabled the rule")
}

// TestFrontMatterKinds_DisablesRule verifies that kinds declared in front
// matter disable the rule for that file.
func TestFrontMatterKinds_DisablesRule(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.md")
	src := "---\nkinds: [quiet]\n---\n# Hello\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(src), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			"quiet": {Rules: map[string]config.RuleCfg{
				"mock-rule": {Enabled: false},
			}},
		},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}

	result := runner.Run([]string{mdFile})
	require.Empty(t, result.Errors)
	assert.Empty(t, result.Diagnostics, "front-matter kind should have disabled the rule")
}

// TestFrontMatterKinds_UndeclaredIsError verifies that a file whose front
// matter references an undeclared kind produces a clear config error.
func TestFrontMatterKinds_UndeclaredIsError(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.md")
	src := "---\nkinds: [ghost]\n---\n# Hello\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(src), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{}, // "ghost" not declared
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}

	result := runner.Run([]string{mdFile})
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "ghost")
}

// TestRunSource_FrontMatterKinds_UndeclaredIsError verifies that RunSource
// also validates front-matter kinds and returns an error for undeclared ones.
func TestRunSource_FrontMatterKinds_UndeclaredIsError(t *testing.T) {
	src := []byte("---\nkinds: [ghost]\n---\n# Hello\n")

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}

	result := runner.RunSource("doc.md", src)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "ghost")
}

// TestRun_FrontMatterKinds_InvalidYAMLIsError verifies that Run returns an
// error when a file's front matter contains invalid YAML (aliases) in kinds.
func TestRun_FrontMatterKinds_InvalidYAMLIsError(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.md")
	src := "---\nbase: &a [plan]\nkinds: *a\n---\n# Hello\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(src), 0o644))

	runner := &Runner{
		Config:           &config.Config{Rules: map[string]config.RuleCfg{}},
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}

	result := runner.Run([]string{mdFile})
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "parsing front-matter kinds")
}

// TestRunSource_FrontMatterKinds_InvalidYAMLIsError verifies that RunSource
// returns an error when front matter contains invalid YAML in the kinds field.
func TestRunSource_FrontMatterKinds_InvalidYAMLIsError(t *testing.T) {
	src := []byte("---\nbase: &a [plan]\nkinds: *a\n---\n# Hello\n")

	runner := &Runner{
		Config:           &config.Config{Rules: map[string]config.RuleCfg{}},
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}

	result := runner.RunSource("doc.md", src)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "parsing front-matter kinds")
}

// TestKindSetsRequiredStructureSchema verifies that a kind setting
// required-structure.schema is reflected in the effective rule config for
// files assigned to that kind.
func TestKindSetsRequiredStructureSchema(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"required-structure": {Enabled: true, Settings: map[string]any{"schema": ""}},
		},
		Kinds: map[string]config.KindBody{
			"plan": {Rules: map[string]config.RuleCfg{
				"required-structure": {Enabled: true, Settings: map[string]any{"schema": "plan/proto.md"}},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}

	effective := config.Effective(cfg, "plan/001_foo.md", nil)
	got := effective["required-structure"].Settings["schema"]
	assert.Equal(t, "plan/proto.md", got)
}
