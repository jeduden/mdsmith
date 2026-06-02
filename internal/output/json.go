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
	// Deprecated and ReplacedBy mirror lint.Diagnostic's plan-136
	// fields so CI scripts can route a deprecation warning without
	// scanning the message body. Both are omitempty so non-
	// deprecation diagnostics stay unchanged on the wire.
	Deprecated bool   `json:"deprecated,omitempty"`
	ReplacedBy string `json:"replaced_by,omitempty"`
	// RelatedLocations mirrors lint.Diagnostic's plan-230 field so CI
	// scripts can read the schema-constraint location without parsing
	// the message. omitempty so diagnostics that carry none stay
	// unchanged on the wire. The rule-doc URL is not emitted here — it
	// is derivable from the `rule` field and is an editor (LSP) concern.
	RelatedLocations []jsonRelatedLocation `json:"related_locations,omitempty"`
}

type jsonRelatedLocation struct {
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
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
			File:             d.File,
			Line:             d.DisplayLine(),
			Column:           d.Column,
			Rule:             d.RuleID,
			Name:             d.RuleName,
			Severity:         string(d.Severity),
			Message:          d.Message,
			SourceLines:      d.SourceLines,
			SourceStartLine:  d.SourceStartLine,
			Explanation:      explanationToJSON(d.Explanation),
			Deprecated:       d.Deprecated,
			ReplacedBy:       d.ReplacedBy,
			RelatedLocations: relatedToJSON(d.RelatedLocations),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

// relatedToJSON maps the structured related locations onto their JSON
// form. Returns nil (so omitempty fires) when there are none.
func relatedToJSON(locs []lint.RelatedLocation) []jsonRelatedLocation {
	if len(locs) == 0 {
		return nil
	}
	out := make([]jsonRelatedLocation, 0, len(locs))
	for _, l := range locs {
		out = append(out, jsonRelatedLocation{
			File: l.File, Line: l.Line, Column: l.Column, Message: l.Message,
		})
	}
	return out
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
