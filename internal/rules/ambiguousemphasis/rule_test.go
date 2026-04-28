package ambiguousemphasis

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// activeRule returns a rule with the "active" defaults (matching plan
// 112's portable / plain profiles): max-run: 2, both bool flags true.
func activeRule() *Rule {
	return &Rule{
		MaxRun:                  2,
		ForbidEscapedInRun:      true,
		ForbidAdjacentSameDelim: true,
	}
}

func runOnLine(t *testing.T, r *Rule, line string) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(line+"\n"))
	require.NoError(t, err)
	return r.Check(f)
}

func TestMetaInformation(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS047", r.ID())
	assert.Equal(t, "ambiguous-emphasis", r.Name())
	assert.Equal(t, "meta", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestDefaultSettings_NoOp(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	diags := runOnLine(t, r, "***bold-italic*** *****\\*a* __a__b__")
	assert.Empty(t, diags, "rule must be a no-op when configured with DefaultSettings")
}

func TestCheck_BoldRunOfTwo_NoDiagnostic(t *testing.T) {
	diags := runOnLine(t, activeRule(), "**bold**")
	assert.Empty(t, diags)
}

func TestCheck_TripleRun_OneDiagnostic(t *testing.T) {
	diags := runOnLine(t, activeRule(), "***bold-italic***")
	require.Len(t, diags, 1)
	assert.Equal(t, "emphasis run of 3 delimiters; max is 2", diags[0].Message)
	assert.Equal(t, 1, diags[0].Column)
}

func TestCheck_RantStringFiveStarsEscape(t *testing.T) {
	diags := runOnLine(t, activeRule(), `*****\*a*`)
	require.Len(t, diags, 2)
	assert.Equal(t, "emphasis run of 5 delimiters; max is 2", diags[0].Message)
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t, "escaped delimiter inside emphasis run", diags[1].Message)
	assert.Equal(t, 6, diags[1].Column)
}

func TestCheck_AdjacentSameDelim(t *testing.T) {
	diags := runOnLine(t, activeRule(), "__a__b__")
	require.Len(t, diags, 1)
	assert.Equal(t, "adjacent same-delimiter emphasis is ambiguous", diags[0].Message)
	assert.Equal(t, 1, diags[0].Column)
}

func TestCheck_AdjacentSingleStar(t *testing.T) {
	diags := runOnLine(t, activeRule(), "*a*b*")
	require.Len(t, diags, 1)
	assert.Equal(t, "adjacent same-delimiter emphasis is ambiguous", diags[0].Message)
}

func TestCheck_RunsSeparatedByWhitespace_NoDiagnostic(t *testing.T) {
	diags := runOnLine(t, activeRule(), "*a* *b* *c*")
	assert.Empty(t, diags)
}

func TestCheck_RantOpenerCloser(t *testing.T) {
	diags := runOnLine(t, activeRule(), "***Peter* Piper**")
	require.Len(t, diags, 1)
	assert.Equal(t, "emphasis run of 3 delimiters; max is 2", diags[0].Message)
}

func TestCheck_BackslashEscapesAreNotRuns(t *testing.T) {
	diags := runOnLine(t, activeRule(), `\*\*\*not-italic`)
	assert.Empty(t, diags, "fully escaped delimiters must not count as runs or escaped-in-run")
}

func TestCheck_CodeSpanSuppresses(t *testing.T) {
	r := activeRule()
	src := []byte("In a code span: `*****\\*a*` here.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	assert.Empty(t, diags, "patterns inside an inline code span must not flag")
}

func TestCheck_FencedCodeBlockSuppresses(t *testing.T) {
	r := activeRule()
	src := []byte("Before.\n\n```\n*****\\*a*\n***triple***\n__a__b__\n```\n\nAfter.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	assert.Empty(t, diags, "patterns inside a fenced code block must not flag")
}

func TestCheck_IndentedCodeBlockSuppresses(t *testing.T) {
	r := activeRule()
	src := []byte("Before.\n\n    *****\\*a*\n    ***triple***\n\nAfter.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	assert.Empty(t, diags, "patterns inside an indented code block must not flag")
}

func TestCheck_LongRunDedup_OnlyOnePerLength(t *testing.T) {
	// `***x***` has two runs of length 3; we expect a single diagnostic
	// per (char, length) so symmetric openers and closers collapse.
	diags := runOnLine(t, activeRule(), "***x***")
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Column)
}

func TestApplySettings_RejectsUnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bogus": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_TypeMismatches(t *testing.T) {
	r := &Rule{}
	cases := map[string]map[string]any{
		"max-run not int":          {"max-run": "two"},
		"max-run negative":         {"max-run": -1},
		"forbid-escaped not bool":  {"forbid-escaped-in-run": "yes"},
		"forbid-adjacent not bool": {"forbid-adjacent-same-delim": 1},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			require.Error(t, r.ApplySettings(s))
		})
	}
}

func TestApplySettings_AcceptsValidValues(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"max-run":                    3,
		"forbid-escaped-in-run":      true,
		"forbid-adjacent-same-delim": true,
	}))
	assert.Equal(t, 3, r.MaxRun)
	assert.True(t, r.ForbidEscapedInRun)
	assert.True(t, r.ForbidAdjacentSameDelim)
}

func TestRule_RegisteredAsConfigurableAndDefaultable(t *testing.T) {
	r := &Rule{}
	var _ rule.Configurable = r
	var _ rule.Defaultable = r
}

func TestScanLine_EmptyLine(t *testing.T) {
	runs, escapes := scanLine([]byte(""))
	assert.Empty(t, runs)
	assert.Empty(t, escapes)
}

func TestScanLine_DoubleBackslashThenStar(t *testing.T) {
	// `\\*` is escaped backslash followed by literal `*`. The `*` is
	// unescaped and should start a run.
	runs, escapes := scanLine([]byte(`\\*`))
	require.Len(t, runs, 1)
	assert.Equal(t, byte('*'), runs[0].char)
	assert.Equal(t, 1, runs[0].length())
	assert.Empty(t, escapes)
}
