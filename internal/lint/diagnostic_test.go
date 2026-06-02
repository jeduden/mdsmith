package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiagnosticFields(t *testing.T) {
	d := Diagnostic{
		File:     "README.md",
		Line:     10,
		Column:   5,
		RuleID:   "MDS001",
		RuleName: "line-length",
		Severity: Error,
		Message:  "line too long (120 > 80)",
	}

	assert.Equal(t, "README.md", d.File)
	assert.Equal(t, 10, d.Line)
	assert.Equal(t, 5, d.Column)
	assert.Equal(t, "MDS001", d.RuleID)
	assert.Equal(t, "line-length", d.RuleName)
	assert.Equal(t, Error, d.Severity)
	assert.Equal(t, "line too long (120 > 80)", d.Message)
}

func TestDiagnosticRelatedLocations(t *testing.T) {
	d := Diagnostic{
		File:     "task.md",
		Line:     1,
		RuleID:   "MDS020",
		RuleName: "required-structure",
		Severity: Error,
		Message:  `status: got "draft", expected one of "open"`,
		RelatedLocations: []RelatedLocation{{
			File:    "plan/proto.md",
			Line:    4,
			Column:  3,
			Message: `schema requires one of: "open", "in-progress"`,
		}},
	}

	if assert.Len(t, d.RelatedLocations, 1) {
		rl := d.RelatedLocations[0]
		assert.Equal(t, "plan/proto.md", rl.File)
		assert.Equal(t, 4, rl.Line)
		assert.Equal(t, 3, rl.Column)
		assert.Equal(t, `schema requires one of: "open", "in-progress"`, rl.Message)
	}
}

func TestDiagnosticZeroValueHasNoRelatedLocations(t *testing.T) {
	// A diagnostic built without the new field keeps the old shape:
	// a nil related-locations slice.
	d := Diagnostic{File: "README.md", Line: 10, RuleID: "MDS001"}
	assert.Nil(t, d.RelatedLocations)
}

func TestSeverityConstants(t *testing.T) {
	assert.Equal(t, Severity("error"), Error)
	assert.Equal(t, Severity("warning"), Warning)
}

func TestLineRange_Contains(t *testing.T) {
	r := LineRange{From: 5, To: 8}
	assert.True(t, r.Contains(5), "start boundary")
	assert.True(t, r.Contains(6), "middle")
	assert.True(t, r.Contains(8), "end boundary")
	assert.False(t, r.Contains(4), "before range")
	assert.False(t, r.Contains(9), "after range")
}

// TestDiagnostic_DisplayLineClamp covers DisplayLine: a non-positive
// sentinel (plan 230's wholly-generated anchor) clamps to 1 for 1-based
// output, while a real line passes through unchanged.
func TestDiagnostic_DisplayLineClamp(t *testing.T) {
	assert.Equal(t, 1, Diagnostic{Line: 0}.DisplayLine(), "zero clamps to 1")
	assert.Equal(t, 1, Diagnostic{Line: -3}.DisplayLine(), "negative clamps to 1")
	assert.Equal(t, 7, Diagnostic{Line: 7}.DisplayLine(), "real line passes through")
}
