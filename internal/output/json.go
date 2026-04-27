package output

import (
	"encoding/json"
	"io"

	"github.com/jeduden/mdsmith/internal/lint"
)

// JSONFormatter outputs diagnostics as a JSON array.
type JSONFormatter struct{}

type jsonDiagnostic struct {
	File            string           `json:"file"`
	Line            int              `json:"line"`
	Column          int              `json:"column"`
	Rule            string           `json:"rule"`
	Name            string           `json:"name"`
	Severity        string           `json:"severity"`
	Message         string           `json:"message"`
	SourceLines     []string         `json:"source_lines,omitempty"`
	SourceStartLine int              `json:"source_start_line,omitempty"`
	Explanation     *jsonExplanation `json:"explanation,omitempty"`
}

type jsonExplanation struct {
	Rule   string                `json:"rule"`
	Leaves []jsonExplanationLeaf `json:"leaves"`
}

type jsonExplanationLeaf struct {
	Path   string `json:"path"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// Format writes diagnostics as a pretty-printed JSON array.
// An empty slice of diagnostics produces [].
func (f *JSONFormatter) Format(w io.Writer, diagnostics []lint.Diagnostic) error {
	items := make([]jsonDiagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		items = append(items, jsonDiagnostic{
			File:            d.File,
			Line:            d.Line,
			Column:          d.Column,
			Rule:            d.RuleID,
			Name:            d.RuleName,
			Severity:        string(d.Severity),
			Message:         d.Message,
			SourceLines:     d.SourceLines,
			SourceStartLine: d.SourceStartLine,
			Explanation:     explanationToJSON(d.Explanation),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func explanationToJSON(e *lint.Explanation) *jsonExplanation {
	if e == nil {
		return nil
	}
	leaves := make([]jsonExplanationLeaf, 0, len(e.Leaves))
	for _, l := range e.Leaves {
		leaves = append(leaves, jsonExplanationLeaf{
			Path: l.Path, Value: l.Value, Source: l.Source,
		})
	}
	return &jsonExplanation{Rule: e.Rule, Leaves: leaves}
}
