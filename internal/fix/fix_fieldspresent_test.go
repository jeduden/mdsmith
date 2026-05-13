package fix

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When a kind-assignment entry uses fields-present:, prepareFile decodes
// the file's front matter into a mapping and the matching kind's rules
// merge into the effective config. The mock rule is disabled at top
// level and enabled by the kind; if the fields-present path doesn't fire
// the rule stays disabled and no pre-fix failure is recorded.
func TestFix_FieldsPresentSelectorAppliesKindRules(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "task.md")
	body := "---\nstatus: open\npriority: high\n---\n# Title  \n"
	require.NoError(t, os.WriteFile(mdFile, []byte(body), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: false},
		},
		Kinds: map[string]config.KindBody{
			"task": {Rules: map[string]config.RuleCfg{
				"mock-trailing": {Enabled: true},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{FieldsPresent: []string{"status", "priority"}, Kinds: []string{"task"}},
		},
	}

	fixer := &Fixer{
		Config:           cfg,
		Rules:            []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		StripFrontMatter: true,
	}
	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)
	require.Equal(t, 1, result.Failures,
		"task kind's fixable rule should fire because fields-present matched")
}

// Without a fields-present selector, prepareFile keeps the kinds-only
// parse path: malformed front matter (a YAML sequence) does not
// generate a parse error. With the selector active the same file
// surfaces the error from parseFieldsForSelector.
func TestFix_FieldsPresentParseErrorGated(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "bad.md")
	body := "---\n- not\n- a\n- mapping\n---\n# Bad\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(body), 0o644))

	cfgNoSelector := &config.Config{
		Rules: map[string]config.RuleCfg{"mock-trailing": {Enabled: true}},
	}
	fNo := &Fixer{
		Config:           cfgNoSelector,
		Rules:            []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		StripFrontMatter: true,
	}
	resNo := fNo.Fix([]string{mdFile})
	assert.Empty(t, resNo.Errors,
		"without fields-present, malformed FM should not surface a parse error")

	cfgSelector := &config.Config{
		Rules: map[string]config.RuleCfg{"mock-trailing": {Enabled: true}},
		Kinds: map[string]config.KindBody{"task": {}},
		KindAssignment: []config.KindAssignmentEntry{
			{FieldsPresent: []string{"status"}, Kinds: []string{"task"}},
		},
	}
	fYes := &Fixer{
		Config:           cfgSelector,
		Rules:            []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		StripFrontMatter: true,
	}
	resYes := fYes.Fix([]string{mdFile})
	require.NotEmpty(t, resYes.Errors,
		"with fields-present, malformed FM should surface a parse error")
	assert.Contains(t, resYes.Errors[0].Error(), "parsing front matter")
}
