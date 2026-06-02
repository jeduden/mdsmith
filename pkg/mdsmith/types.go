package mdsmith

import (
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/kindsout"
	"github.com/jeduden/mdsmith/internal/lint"
)

// Diagnostic is one lint finding. Its JSON shape matches the
// `--format json` CLI and the LSP server, so a JavaScript host decodes
// it without a second schema. Column is a 1-based UTF-16 code-unit
// offset (the LSP default), measured once on the Go side.
type Diagnostic struct {
	File            string       `json:"file"`
	Line            int          `json:"line"`
	Column          int          `json:"column"`
	Rule            string       `json:"rule"`
	Name            string       `json:"name"`
	Severity        string       `json:"severity"`
	Message         string       `json:"message"`
	SourceLines     []string     `json:"source_lines,omitempty"`
	SourceStartLine int          `json:"source_start_line,omitempty"`
	Explanation     *Explanation `json:"explanation,omitempty"`
	Deprecated      bool         `json:"deprecated,omitempty"`
	ReplacedBy      string       `json:"replaced_by,omitempty"`

	// RelatedLocations carries secondary source locations — for MDS020,
	// the schema constraint a value violated — matching the CLI
	// `--format json` shape so the WASM/Session host and the CLI decode
	// one schema. A host (e.g. the Obsidian plugin) renders these as
	// navigable links. Omitted when there are none.
	RelatedLocations []RelatedLocation `json:"related_locations,omitempty"`

	// RuleID is an unexported-on-the-wire convenience for Go callers
	// that want the rule identifier without parsing JSON. It is not
	// serialized; use Rule for the wire field.
	RuleID string `json:"-"`
}

// RelatedLocation is a secondary source location attached to a
// Diagnostic — for a schema violation, the line of the schema
// constraint. File may differ from the owning Diagnostic.File (the
// schema lives in a separate proto.md, kind file, or config).
type RelatedLocation struct {
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// Explanation attaches per-leaf rule provenance to a Diagnostic. It is
// populated only when the session is asked to explain (a future
// capability); today it is always nil and omitted on the wire.
type Explanation struct {
	Rule   string            `json:"rule"`
	Leaves []ExplanationLeaf `json:"leaves"`
}

// ExplanationLeaf is one effective config leaf and the layer that set
// it.
type ExplanationLeaf struct {
	Path   string `json:"path"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// FixResult is the outcome of [Session.Fix]: the rewritten source, a
// changed flag, and the diagnostics that remain after fixing.
type FixResult struct {
	// Source is the rewritten document (front matter included). Equals
	// the input when no rule produced an edit.
	Source string `json:"source"`
	// Changed reports whether Source differs from the input bytes.
	Changed bool `json:"changed"`
	// Diagnostics are the findings that survive the fix — non-fixable
	// rules and any violations that could not be auto-fixed.
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// KindResolution is the per-file kind list and effective rule
// configuration with per-leaf provenance. Its JSON shape matches the
// `mdsmith kinds resolve --json` CLI output.
type KindResolution = kindsout.FileResolutionJSON

// toDiagnostics converts engine diagnostics to the public shape,
// measuring UTF-16 columns once here so JS callers get LSP-native
// offsets.
func toDiagnostics(in []lint.Diagnostic) []Diagnostic {
	if len(in) == 0 {
		return nil
	}
	out := make([]Diagnostic, 0, len(in))
	for _, d := range in {
		out = append(out, Diagnostic{
			File:             d.File,
			Line:             d.DisplayLine(),
			Column:           d.Column,
			Rule:             d.RuleID,
			RuleID:           d.RuleID,
			Name:             d.RuleName,
			Severity:         string(d.Severity),
			Message:          d.Message,
			SourceLines:      d.SourceLines,
			SourceStartLine:  d.SourceStartLine,
			Explanation:      toExplanation(d.Explanation),
			Deprecated:       d.Deprecated,
			ReplacedBy:       d.ReplacedBy,
			RelatedLocations: toRelatedLocations(d.RelatedLocations),
		})
	}
	return out
}

// toRelatedLocations converts engine related locations to the public
// shape; returns nil for none so the JSON field is omitted.
func toRelatedLocations(in []lint.RelatedLocation) []RelatedLocation {
	if len(in) == 0 {
		return nil
	}
	out := make([]RelatedLocation, 0, len(in))
	for _, l := range in {
		out = append(out, RelatedLocation{
			File: l.File, Line: l.Line, Column: l.Column, Message: l.Message,
		})
	}
	return out
}

func toExplanation(e *lint.Explanation) *Explanation {
	if e == nil {
		return nil
	}
	leaves := make([]ExplanationLeaf, 0, len(e.Leaves))
	for _, l := range e.Leaves {
		leaves = append(leaves, ExplanationLeaf{
			Path:   l.Path,
			Value:  l.Value,
			Source: l.Source,
		})
	}
	return &Explanation{Rule: e.Rule, Leaves: leaves}
}

// toKindResolution converts a config.FileResolution to the public JSON
// shape, reusing the same converter the CLI's `kinds resolve --json`
// uses so the wire format stays identical across surfaces.
func toKindResolution(res *config.FileResolution) KindResolution {
	return kindsout.FileResolution(res)
}
