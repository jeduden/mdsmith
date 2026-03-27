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

func TestSeverityConstants(t *testing.T) {
	assert.Equal(t, Severity("error"), Error)
	assert.Equal(t, Severity("warning"), Warning)
}
