package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When a kind-assignment entry uses fields-present:, the runner decodes
// the file's full front matter and the entry's kind appears in the
// effective rule resolution. Without this path being exercised the new
// parseFrontMatterFields wrapper only ran its short-circuit branch.
func TestRunner_FieldsPresentSelectorAppliesKind(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "task.md")
	body := "---\nstatus: open\npriority: high\n---\n# Task\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(body), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: false},
		},
		Kinds: map[string]config.KindBody{
			"task": {Rules: map[string]config.RuleCfg{
				"mock-rule": {Enabled: true},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{FieldsPresent: []string{"status", "priority"}, Kinds: []string{"task"}},
		},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}

	result := runner.Run([]string{mdFile})
	require.Empty(t, result.Errors)
	require.Len(t, result.Diagnostics, 1,
		"task kind selected via fields-present should enable mock-rule")
}

// A file whose front matter cannot be decoded as a mapping surfaces a
// parse error only when an entry actually uses the fields-present
// selector. The first run (no fields-present entry) succeeds; the
// second run errors.
func TestRunner_FieldsPresentParseErrorGated(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "bad.md")
	// Sequence at the top of FM — not a mapping. ParseFrontMatterKinds'
	// fast path skips it (no "kinds:" substring); ParseFrontMatterFields
	// rejects it.
	body := "---\n- not\n- a\n- mapping\n---\n# Bad\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(body), 0o644))

	cfgNoSelector := &config.Config{
		Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}},
	}
	rNo := &Runner{
		Config:           cfgNoSelector,
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}
	resNo := rNo.Run([]string{mdFile})
	assert.Empty(t, resNo.Errors,
		"without fields-present, malformed FM should not be parsed")

	cfgSelector := &config.Config{
		Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}},
		Kinds: map[string]config.KindBody{"task": {}},
		KindAssignment: []config.KindAssignmentEntry{
			{FieldsPresent: []string{"status"}, Kinds: []string{"task"}},
		},
	}
	rYes := &Runner{
		Config:           cfgSelector,
		Rules:            []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		StripFrontMatter: true,
	}
	resYes := rYes.Run([]string{mdFile})
	require.NotEmpty(t, resYes.Errors,
		"with fields-present, malformed FM should surface a parse error")
	assert.Contains(t, resYes.Errors[0].Error(), "parsing front matter")
}
