package descriptivelinktext

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{Banned: append([]string(nil), defaultBanned...)}
	return r.Check(f)
}

func TestDescriptiveText(t *testing.T) {
	diags := check(t, "# T\n\n[the install guide](x)\n")
	assert.Empty(t, diags)
}

func TestClickHere(t *testing.T) {
	diags := check(t, "# T\n\n[click here](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "click here" is not descriptive`, diags[0].Message)
}

func TestHere(t *testing.T) {
	diags := check(t, "# T\n\nSee [here](x) for details.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "here" is not descriptive`, diags[0].Message)
}

func TestLink(t *testing.T) {
	diags := check(t, "# T\n\n[link](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "link" is not descriptive`, diags[0].Message)
}

func TestMore(t *testing.T) {
	diags := check(t, "# T\n\n[more](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "more" is not descriptive`, diags[0].Message)
}

func TestCaseInsensitive(t *testing.T) {
	diags := check(t, "# T\n\n[Click Here](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "Click Here" is not descriptive`, diags[0].Message)
}

func TestWhitespaceInsensitive(t *testing.T) {
	diags := check(t, "# T\n\n[click  here](x)\n")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "not descriptive")
}

func TestCodeSpanOnly(t *testing.T) {
	diags := check(t, "# T\n\n[`here`](x)\n")
	assert.Empty(t, diags)
}

func TestImageOnly(t *testing.T) {
	diags := check(t, "# T\n\n[![alt](img.png)](x)\n")
	assert.Empty(t, diags)
}

func TestCustomBannedReplaces(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# T\n\n[click here](x)\n\n[read more](y)\n"))
	require.NoError(t, err)
	r := &Rule{Banned: []string{"read more"}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "read more" is not descriptive`, diags[0].Message)
}

func TestEmptyBannedList(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# T\n\n[click here](x)\n"))
	require.NoError(t, err)
	r := &Rule{Banned: []string{}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestLineNumber(t *testing.T) {
	diags := check(t, "# T\n\nSome text.\n\n[here](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
}

func TestApplySettingsBanned(t *testing.T) {
	r := &Rule{Banned: append([]string(nil), defaultBanned...)}
	err := r.ApplySettings(map[string]any{
		"banned": []any{"read more", "learn more"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"read more", "learn more"}, r.Banned)
}

func TestApplySettingsUnknown(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "x"})
	assert.ErrorContains(t, err, "unknown setting")
}

func TestApplySettingsBannedWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"banned": "not-a-list"})
	assert.ErrorContains(t, err, "list of strings")
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault(), "MDS063 must be opt-in")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	banned, ok := s["banned"].([]string)
	require.True(t, ok)
	assert.Equal(t, defaultBanned, banned)
}

func TestEmphasisWrappedBannedText(t *testing.T) {
	diags := check(t, "# T\n\n[*here*](x)\n")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "not descriptive")
}

func TestSoftLineBreakInLinkText(t *testing.T) {
	diags := check(t, "# T\n\n[click\nhere](x)\n")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "not descriptive")
}
