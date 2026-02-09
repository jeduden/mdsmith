package output

import (
	"encoding/json"
	"io"

	"github.com/jeduden/tidymark/internal/lint"
)

// JSONFormatter outputs diagnostics as a JSON array.
type JSONFormatter struct{}

type jsonDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Rule     string `json:"rule"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Format writes diagnostics as a pretty-printed JSON array.
// An empty slice of diagnostics produces [].
func (f *JSONFormatter) Format(w io.Writer, diagnostics []lint.Diagnostic) error {
	items := make([]jsonDiagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		items = append(items, jsonDiagnostic{
			File:     d.File,
			Line:     d.Line,
			Column:   d.Column,
			Rule:     d.RuleID,
			Name:     d.RuleName,
			Severity: string(d.Severity),
			Message:  d.Message,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}
