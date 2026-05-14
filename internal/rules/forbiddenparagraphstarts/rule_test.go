package forbiddenparagraphstarts

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
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
	assert.Equal(t, "MDS055", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "forbidden-paragraph-starts", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "prose", (&Rule{}).Category())
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestCheck_NoSettings_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	assert.Empty(t, r.Check(mustFile(t, "We must do this.\n")))
}

func TestCheck_StartsMatch_Diagnostic(t *testing.T) {
	r := &Rule{Starts: []string{"We "}}
	diags := r.Check(mustFile(t, "We must do this.\n"))
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS055", diags[0].RuleID)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, `"We "`)
	assert.Contains(t, diags[0].Message, "forbidden prefix")
}

func TestCheck_StartsNoMatch_NoDiagnostic(t *testing.T) {
	r := &Rule{Starts: []string{"We "}}
	assert.Empty(t, r.Check(mustFile(t, "The team must do this.\n")))
}

func TestCheck_OnlyFirstMatchEmitted(t *testing.T) {
	r := &Rule{Starts: []string{"The ", "The team"}}
	diags := r.Check(mustFile(t, "The team must do this.\n"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "The ")
}

func TestCheck_MultipleParagraphs_EachChecked(t *testing.T) {
	src := "We start with this.\n\nThe team does that.\n\nWe end with this.\n"
	r := &Rule{Starts: []string{"We "}}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 2)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 5, diags[1].Line)
}

func TestCheck_NilAST_NoDiagnostic(t *testing.T) {
	r := &Rule{Starts: []string{"We"}}
	assert.Empty(t, r.Check(&lint.File{}))
}

func TestCheck_EmptyPrefixIgnored(t *testing.T) {
	r := &Rule{Starts: []string{""}}
	assert.Empty(t, r.Check(mustFile(t, "Any paragraph.\n")))
}

func TestCheck_TableParagraphSkipped(t *testing.T) {
	// Goldmark parses tables as paragraphs when the table extension is
	// absent; the rule must not match the pipe-prefixed text.
	src := "| col |\n| --- |\n| val |\n"
	r := &Rule{Starts: []string{"|"}}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestApplySettings_Starts(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"starts": []any{"We", "The "},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"We", "The "}, r.Starts)
}

func TestApplySettings_InvalidStarts(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"starts": 42})
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
	assert.Equal(t, []string{}, ds["starts"])
}

func TestApplyDefaultSettings_ClearsStarts(t *testing.T) {
	r := &Rule{Starts: []string{"We"}}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	assert.Empty(t, r.Starts)
}
