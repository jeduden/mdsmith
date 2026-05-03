package propernames

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func ruleWith(names ...string) *Rule {
	return &Rule{Names: names}
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS050", r.ID())
	assert.Equal(t, "proper-names", r.Name())
	assert.Equal(t, "prose", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestCheck_CorrectCasing_NoDiagnostic(t *testing.T) {
	f := newFile(t, "JavaScript is fun.\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_WrongCasing_Diagnostic(t *testing.T) {
	f := newFile(t, "Javascript is fun.\n")
	diags := ruleWith("JavaScript").Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "Javascript" should be "JavaScript"`, diags[0].Message)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
}

func TestCheck_AllCaps_Diagnostic(t *testing.T) {
	f := newFile(t, "JAVASCRIPT is a language.\n")
	diags := ruleWith("JavaScript").Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "JAVASCRIPT" should be "JavaScript"`, diags[0].Message)
}

func TestCheck_Fix_ReplacesWrongCasing(t *testing.T) {
	f := newFile(t, "Use javascript and github.\n")
	r := &Rule{Names: []string{"JavaScript", "GitHub"}}
	fixed := r.Fix(f)
	assert.Equal(t, "Use JavaScript and GitHub.\n", string(fixed))
}

// TestCheck_TrailingLetterWrongCasing: "Javascripts" has "Javascript" (10 chars,
// wrong case) at position 0. Only the left boundary is required, so the trailing
// 's' does not prevent the match.
func TestCheck_TrailingLetterWrongCasing(t *testing.T) {
	f := newFile(t, "Javascripts are popular.\n")
	diags := ruleWith("JavaScript").Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "Javascript" should be "JavaScript"`, diags[0].Message)
}

// TestCheck_GitHubberNoMatch: "GitHubber" has "GitHub" (correctly cased) at
// position 0. No diagnostic because the matched text has the right casing.
func TestCheck_GitHubberNoMatch(t *testing.T) {
	f := newFile(t, "GitHubber contributes code.\n")
	diags := ruleWith("GitHub").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_Heading_Diagnostic(t *testing.T) {
	f := newFile(t, "# Github\n\nSome text.\n")
	diags := ruleWith("GitHub").Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "Github" should be "GitHub"`, diags[0].Message)
}

func TestCheck_LinkText_Diagnostic(t *testing.T) {
	// Link text is checked but the destination URL is not.
	f := newFile(t, "[Github](https://github.com)\n")
	diags := ruleWith("GitHub").Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "Github" should be "GitHub"`, diags[0].Message)
}

func TestCheck_LinkURL_NotChecked(t *testing.T) {
	// Correctly-cased link text with a URL that contains "github": no diagnostic.
	f := newFile(t, "[GitHub](https://github.com)\n")
	diags := ruleWith("GitHub").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_CodeSpan_SkippedByDefault(t *testing.T) {
	f := newFile(t, "Use `javascript` today.\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_CodeSpan_CheckedWhenEnabled(t *testing.T) {
	f := newFile(t, "Use `javascript` today.\n")
	r := &Rule{Names: []string{"JavaScript"}, CheckCode: true}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "javascript" should be "JavaScript"`, diags[0].Message)
}

func TestCheck_FencedCodeBlock_SkippedByDefault(t *testing.T) {
	f := newFile(t, "```\njavascript code\n```\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_FencedCodeBlock_CheckedWhenEnabled(t *testing.T) {
	f := newFile(t, "```\njavascript code\n```\n")
	r := &Rule{Names: []string{"JavaScript"}, CheckCode: true}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "javascript" should be "JavaScript"`, diags[0].Message)
}

func TestCheck_NoNames_NoDiagnostic(t *testing.T) {
	f := newFile(t, "javascript github\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MiddleOfWord_NotMatched(t *testing.T) {
	// "myjavascript" — left boundary 'y' is a word char → no match.
	f := newFile(t, "myjavascript is not a match.\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_AfterPunctuation_Matched(t *testing.T) {
	f := newFile(t, "Use: javascript, typescript.\n")
	diags := ruleWith("JavaScript").Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "javascript" should be "JavaScript"`, diags[0].Message)
}

func TestCheck_AfterHyphen_Matched(t *testing.T) {
	f := newFile(t, "pre-javascript era.\n")
	diags := ruleWith("JavaScript").Check(f)
	require.Len(t, diags, 1)
}

func TestCheck_AfterDot_Matched(t *testing.T) {
	f := newFile(t, "end.javascript start.\n")
	diags := ruleWith("JavaScript").Check(f)
	require.Len(t, diags, 1)
}

func TestCheck_AutoLink_NotChecked(t *testing.T) {
	// Autolinks like <https://javascript.com> are skipped entirely.
	f := newFile(t, "<https://javascript.com>\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_HTMLBlock_SkippedByDefault(t *testing.T) {
	f := newFile(t, "<div>\njavascript\n</div>\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_HTMLBlock_CheckedWhenEnabled(t *testing.T) {
	f := newFile(t, "<div>\njavascript\n</div>\n")
	r := &Rule{Names: []string{"JavaScript"}, CheckHTML: true}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `proper name "javascript" should be "JavaScript"`, diags[0].Message)
}

func TestCheck_RawHTML_SkippedByDefault(t *testing.T) {
	f := newFile(t, `Some <span id="javascript">text</span>.`+"\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_RawHTML_CheckedWhenEnabled(t *testing.T) {
	// check-html: true causes RawHTML node segments to be scanned;
	// the "javascript" inside the attribute is flagged.
	f := newFile(t, `Some <span id="javascript">text</span>.`+"\n")
	r := &Rule{Names: []string{"JavaScript"}, CheckHTML: true}
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	assert.Equal(t, `proper name "javascript" should be "JavaScript"`, diags[0].Message)
}

func TestFix_NoMatches_ReturnsCopy(t *testing.T) {
	src := "JavaScript is correct.\n"
	f := newFile(t, src)
	out := ruleWith("JavaScript").Fix(f)
	assert.Equal(t, src, string(out))
	// Must be a copy, not the same slice.
	assert.NotSame(t, &f.Source[0], &out[0])
}

func TestFix_OverlappingMatches_LongestMatchWins(t *testing.T) {
	// "Java" (4 chars) and "JavaScript" (10 chars) both match at offset 0.
	// The tie-break prefers the longest match, so "JavaScript" wins and
	// the shorter "Java" is skipped as an overlap.
	f := newFile(t, "javascript is fun.\n")
	r := &Rule{Names: []string{"Java", "JavaScript"}}
	out := r.Fix(f)
	assert.Equal(t, "JavaScript is fun.\n", string(out))
}

func TestScanBytes_EmptyName_Skipped(t *testing.T) {
	r := &Rule{Names: []string{""}}
	f := newFile(t, "javascript\n")
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestScanBytes_NameLongerThanText_Skipped(t *testing.T) {
	r := &Rule{Names: []string{"AVeryLongProperNameThatIsLongerThanAnyText"}}
	f := newFile(t, "hi\n")
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestScanBytes_EmptyText_Skipped(t *testing.T) {
	r := &Rule{Names: []string{"JavaScript"}}
	results := r.scanBytes([]byte{}, 0, []byte{})
	assert.Nil(t, results)
}

func TestScanCodeSpanChildren_NonTextChild_Skipped(t *testing.T) {
	// A CodeSpan whose only child is not *ast.Text — the branch must be skipped without panic.
	f := newFile(t, "test\n")
	r := &Rule{Names: []string{"JavaScript"}}
	cs := ast.NewCodeSpan()
	// ast.NewString is an inline node that is not *ast.Text, making the !ok branch reachable.
	cs.AppendChild(cs, ast.NewString([]byte("javascript")))
	out := r.scanCodeSpanChildren(cs, f)
	assert.Nil(t, out)
}

func TestApplySettings_InvalidCheckHTML(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"check-html": "yes"})
	assert.Error(t, err)
}

func TestCheck_IndentedCodeBlock_SkippedByDefault(t *testing.T) {
	// Indented code blocks are also CodeBlock nodes.
	f := newFile(t, "    javascript code\n")
	diags := ruleWith("JavaScript").Check(f)
	assert.Empty(t, diags)
}

func TestCheck_IndentedCodeBlock_CheckedWhenEnabled(t *testing.T) {
	f := newFile(t, "    javascript code\n")
	r := &Rule{Names: []string{"JavaScript"}, CheckCode: true}
	diags := r.Check(f)
	require.Len(t, diags, 1)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["names"])
	assert.Equal(t, false, ds["check-code"])
	assert.Equal(t, false, ds["check-html"])
}

func TestSettingMergeMode(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("names"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("check-code"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}

func TestApplySettings_Names(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"names": []any{"JavaScript", "GitHub"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"JavaScript", "GitHub"}, r.Names)
}

func TestApplySettings_CheckCode(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"check-code": true})
	require.NoError(t, err)
	assert.True(t, r.CheckCode)
}

func TestApplySettings_CheckHTML(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"check-html": true})
	require.NoError(t, err)
	assert.True(t, r.CheckHTML)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bad-key": "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_InvalidNames(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"names": "not-a-list"})
	assert.Error(t, err)
}

func TestApplySettings_InvalidCheckCode(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"check-code": "yes"})
	assert.Error(t, err)
}
