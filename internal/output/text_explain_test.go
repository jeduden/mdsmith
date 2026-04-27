package output

import (
	"bytes"
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
