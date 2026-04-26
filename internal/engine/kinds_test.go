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

	effective := config.Effective(cfg, "plan/001_foo.md")
	got := effective["required-structure"].Settings["schema"]
	assert.Equal(t, "plan/proto.md", got)
}
