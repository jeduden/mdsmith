package forbiddentext

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestID(t *testing.T) {
	assert.Equal(t, "MDS056", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "forbidden-text", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "prose", (&Rule{}).Category())
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestCheck_NoSettings_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	assert.Empty(t, r.Check(mustFile(t, "we should do this.\n")))
}

func TestCheck_ContainsMatch_Diagnostic(t *testing.T) {
	r := &Rule{Contains: []string{"should"}}
	diags := r.Check(mustFile(t, "we should do this.\n"))
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS056", diags[0].RuleID)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, `"should"`)
}

func TestCheck_ContainsNoMatch_NoDiagnostic(t *testing.T) {
	r := &Rule{Contains: []string{"should"}}
	assert.Empty(t, r.Check(mustFile(t, "we must do this.\n")))
}

func TestCheck_MultipleMatchesPerParagraph(t *testing.T) {
	// Both forbidden substrings appear: each fires its own diagnostic.
	src := "we should may consider it.\n"
	r := &Rule{Contains: []string{"should", "may"}}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, "should")
	assert.Contains(t, diags[1].Message, "may")
}

func TestCheck_MultipleParagraphs(t *testing.T) {
	src := "first should be done.\n\nthe end.\n\nwe may try later.\n"
	r := &Rule{Contains: []string{"should", "may"}}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 2)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 5, diags[1].Line)
}

func TestCheck_NilAST_NoDiagnostic(t *testing.T) {
	r := &Rule{Contains: []string{"x"}}
	assert.Empty(t, r.Check(&lint.File{}))
}

func TestCheck_EmptyContainsIgnored(t *testing.T) {
	r := &Rule{Contains: []string{""}}
	assert.Empty(t, r.Check(mustFile(t, "any text.\n")))
}

func TestCheck_TableParagraphSkipped(t *testing.T) {
	src := "| col |\n| --- |\n| should |\n"
	r := &Rule{Contains: []string{"should"}}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestApplySettings_Contains(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"contains": []any{"should", "may"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"should", "may"}, r.Contains)
}

func TestApplySettings_InvalidContains(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"contains": 42})
	assert.Error(t, err)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	assert.Error(t, err)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, []string{}, ds["contains"])
}

func TestApplyDefaultSettings_ClearsContains(t *testing.T) {
	r := &Rule{Contains: []string{"should"}}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	assert.Empty(t, r.Contains)
}

func TestSettingMergeMode(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("contains"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}
