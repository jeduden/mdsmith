package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextFormatter_ExplainTrailerPlain(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "line-length",
		Severity: lint.Error, Message: "too long",
		Explanation: &lint.Explanation{
			Rule: "line-length",
			Leaves: []lint.ExplanationLeaf{
				{Path: "enabled", Value: true, Source: "default"},
				{Path: "settings.max", Value: 30, Source: "kinds.short"},
			},
		},
	}}
	require.NoError(t, f.Format(&buf, diags))

	out := buf.String()
	assert.Contains(t, out, "└─ line-length:")
	assert.Contains(t, out, "enabled=true (default)")
	assert.Contains(t, out, "settings.max=30 (kinds.short)")
	assert.NotContains(t, out, "\033[", "no color codes when Color is false")
}

func TestTextFormatter_ExplainTrailerColor(t *testing.T) {
	f := &TextFormatter{Color: true}
	var buf bytes.Buffer

	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "line-length",
		Severity: lint.Error, Message: "too long",
		Explanation: &lint.Explanation{
			Rule:   "line-length",
			Leaves: []lint.ExplanationLeaf{{Path: "enabled", Value: true, Source: "default"}},
		},
	}}
	require.NoError(t, f.Format(&buf, diags))

	out := buf.String()
	// dim ANSI sequence around the trailer body
	assert.Contains(t, out, "\033[2m")
	assert.Contains(t, out, "line-length:")
	assert.Contains(t, out, "└─")
}

func TestTextFormatter_ExplainOmittedWhenNil(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "line-length",
		Severity: lint.Error, Message: "too long",
	}}
	require.NoError(t, f.Format(&buf, diags))
	assert.NotContains(t, buf.String(), "└─")
}

func TestFormatLeafValue_UnmarshalableFallsBackToFmt(t *testing.T) {
	// channels are not JSON-encodable; formatLeafValue falls back to %v.
	out := formatLeafValue(make(chan int))
	assert.NotEmpty(t, out)
}

func TestFormatLeafValue_Scalars(t *testing.T) {
	assert.Equal(t, "30", formatLeafValue(30))
	assert.Equal(t, "true", formatLeafValue(true))
	assert.Equal(t, "null", formatLeafValue(nil))
}

func TestTextFormatter_ExplainEmptyLeavesPrintsNoSettings(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "rule",
		Severity: lint.Error, Message: "msg",
		Explanation: &lint.Explanation{Rule: "rule"},
	}}
	require.NoError(t, f.Format(&buf, diags))
	assert.Contains(t, buf.String(), "(no settings)")
}

// TestTextFormatter_ExplainSanitizesControlChars makes sure that
// rule/leaf/source values from user-controlled YAML cannot break the
// single-line trailer with newlines or inject terminal escape codes.
func TestTextFormatter_ExplainSanitizesControlChars(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "evil",
		Severity: lint.Error, Message: "msg",
		Explanation: &lint.Explanation{
			Rule: "evil\nrule\x1b[31m",
			Leaves: []lint.ExplanationLeaf{
				{
					Path:   "settings.bad\x07key",
					Value:  "v\x1b[31m",
					Source: "kinds.bad\nkind",
				},
			},
		},
	}}
	require.NoError(t, f.Format(&buf, diags))

	out := buf.String()
	// One trailer line; no embedded newlines, no ESC, no BEL after split.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var trailer string
	for _, ln := range lines {
		if strings.Contains(ln, "└─") {
			trailer = ln
			break
		}
	}
	require.NotEmpty(t, trailer, "trailer line not found in %q", out)
	assert.NotContains(t, trailer, "\x1b")
	assert.NotContains(t, trailer, "\x07")
	// Sanitized rule/leaf/source content remains visible.
	assert.Contains(t, trailer, "evilrule")
	assert.Contains(t, trailer, "settings.badkey")
	assert.Contains(t, trailer, "kinds.badkind")
}
