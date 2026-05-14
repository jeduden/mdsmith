package requiredmentions

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
	assert.Equal(t, "MDS058", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "required-mentions", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "prose", (&Rule{}).Category())
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestCheck_NoMentions_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	assert.Empty(t, r.Check(mustFile(t, "# Title\n\nbody.\n")))
}

func TestCheck_AllMentionsPresent_NoDiagnostic(t *testing.T) {
	r := &Rule{Mentions: []string{"production", "rollback"}}
	src := "# Title\n\nDeploy to production. Have a rollback ready.\n"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_MissingMention_Diagnostic(t *testing.T) {
	r := &Rule{Mentions: []string{"production"}}
	diags := r.Check(mustFile(t, "# Title\n\nDeploy to staging.\n"))
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS058", diags[0].RuleID)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, `"production"`)
}

func TestCheck_MultipleMissingMentions_MultipleDiagnostics(t *testing.T) {
	r := &Rule{Mentions: []string{"production", "rollback"}}
	diags := r.Check(mustFile(t, "# Title\n\nDeploy to staging.\n"))
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, "production")
	assert.Contains(t, diags[1].Message, "rollback")
}

func TestCheck_NestedSectionContributesToParent(t *testing.T) {
	src := "# A\n\n## B\n\nproduction lives here.\n"
	r := &Rule{Mentions: []string{"production"}}
	// A's body includes B's content. Both match.
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_NoHeadings_NoDiagnostic(t *testing.T) {
	r := &Rule{Mentions: []string{"production"}}
	assert.Empty(t, r.Check(mustFile(t, "no heading here.\n")))
}

func TestCheck_NilAST_NoDiagnostic(t *testing.T) {
	r := &Rule{Mentions: []string{"x"}}
	assert.Empty(t, r.Check(&lint.File{}))
}

func TestCheck_EmptyMentionIgnored(t *testing.T) {
	r := &Rule{Mentions: []string{""}}
	assert.Empty(t, r.Check(mustFile(t, "# Title\n\nbody.\n")))
}

func TestApplySettings_Mentions(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"mentions": []any{"production", "rollback"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"production", "rollback"}, r.Mentions)
}

func TestApplySettings_InvalidMentions(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"mentions": 42})
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
	assert.Equal(t, []string{}, ds["mentions"])
}

func TestApplyDefaultSettings_ClearsMentions(t *testing.T) {
	r := &Rule{Mentions: []string{"foo"}}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	assert.Empty(t, r.Mentions)
}
