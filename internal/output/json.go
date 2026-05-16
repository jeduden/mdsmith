package output

import (
	"encoding/json"
	"io"

	"github.com/jeduden/mdsmith/internal/lint"
)

// JSONFormatter outputs diagnostics as a JSON array.
type JSONFormatter struct{}

// JSONRecord is the exported shape of a single diagnostic in JSON output.
// It is used by both JSONFormatter and the dry-run JSON output.
type JSONRecord struct {
	File            string           `json:"file"`
	Line            int              `json:"line"`
	Column          int              `json:"column"`
	Rule            string           `json:"rule"`
	Name            string           `json:"name"`
	Severity        string           `json:"severity"`
	Message         string           `json:"message"`
	SourceLines     []string         `json:"source_lines,omitempty"`
	SourceStartLine int              `json:"source_start_line,omitempty"`
	Explanation     *JSONExplanation `json:"explanation,omitempty"`
}

// JSONExplanation is the exported shape of a diagnostic explanation.
type JSONExplanation struct {
	Rule   string                `json:"rule"`
	Leaves []JSONExplanationLeaf `json:"leaves"`
}

// JSONExplanationLeaf is one leaf in a diagnostic explanation.
type JSONExplanationLeaf struct {
	Path   string `json:"path"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// DiagnosticsToJSON converts a slice of lint.Diagnostic to []JSONRecord.
func DiagnosticsToJSON(diagnostics []lint.Diagnostic) []JSONRecord {
	if len(diagnostics) == 0 {
		return []JSONRecord{}
	}
	items := make([]JSONRecord, 0, len(diagnostics))
	for _, d := range diagnostics {
		items = append(items, JSONRecord{
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
	return items
}

// Format writes diagnostics as a pretty-printed JSON array.
// An empty slice of diagnostics produces [].
func (f *JSONFormatter) Format(w io.Writer, diagnostics []lint.Diagnostic) error {
	items := DiagnosticsToJSON(diagnostics)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func explanationToJSON(e *lint.Explanation) *JSONExplanation {
	if e == nil {
		return nil
	}
	leaves := make([]JSONExplanationLeaf, 0, len(e.Leaves))
	for _, l := range e.Leaves {
		leaves = append(leaves, JSONExplanationLeaf{
			Path: l.Path, Value: l.Value, Source: l.Source,
		})
	}
	return &JSONExplanation{Rule: e.Rule, Leaves: leaves}
}
