package output

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

const (
	sarifSchemaURI = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion21 = "2.1.0"
	sarifRuleBase  = "https://mdsmith.dev/rules/"
	sarifInfoURI   = "https://mdsmith.dev"
)

// SARIFFormatter emits diagnostics as a SARIF 2.1.0 JSON document.
// ToolVersion is the mdsmith build version stamped into the driver
// metadata; it may be empty for development builds.
type SARIFFormatter struct {
	ToolVersion string
}

// Format writes diagnostics as a SARIF 2.1.0 document to w.
func (f *SARIFFormatter) Format(w io.Writer, diags []lint.Diagnostic) error {
	doc := f.buildSARIFDoc(diags)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func (f *SARIFFormatter) buildSARIFDoc(diags []lint.Diagnostic) sarifDoc21 {
	rules, ruleIndex := buildSARIFRules(diags)
	results := make([]sarifResult21, 0, len(diags))
	for i := range diags {
		results = append(results, buildSARIFResult(&diags[i], ruleIndex))
	}
	return sarifDoc21{
		Schema:  sarifSchemaURI,
		Version: sarifVersion21,
		Runs: []sarifRun21{{
			Tool: sarifTool21{Driver: sarifDriver21{
				Name:           "mdsmith",
				Version:        f.ToolVersion,
				InformationURI: sarifInfoURI,
				Rules:          rules,
			}},
			Results: results,
		}},
	}
}

// buildSARIFRules collects one rule entry per unique RuleID (first
// occurrence wins) and returns the id→index map for ruleIndex fields.
func buildSARIFRules(diags []lint.Diagnostic) ([]sarifRule21, map[string]int) {
	rules := make([]sarifRule21, 0, len(diags))
	index := make(map[string]int, len(diags))
	for i := range diags {
		d := &diags[i]
		if _, seen := index[d.RuleID]; seen {
			continue
		}
		index[d.RuleID] = len(rules)
		rules = append(rules, sarifRule21{
			ID:               d.RuleID,
			HelpURI:          sarifHelpURI(d),
			ShortDescription: sarifText21{Text: d.RuleName},
		})
	}
	return rules, index
}

// sarifHelpURI constructs the mdsmith.dev doc-page URL for a rule.
// Pattern: https://mdsmith.dev/rules/mds001-line-length/
func sarifHelpURI(d *lint.Diagnostic) string {
	if d.RuleID == "" {
		return ""
	}
	slug := strings.ToLower(d.RuleID)
	if d.RuleName != "" {
		slug += "-" + d.RuleName
	}
	return sarifRuleBase + slug + "/"
}

// sarifLevelFor maps mdsmith severity onto the SARIF level vocabulary.
func sarifLevelFor(s lint.Severity) string {
	switch s {
	case lint.Error:
		return "error"
	case lint.Warning:
		return "warning"
	default:
		return "note"
	}
}

// buildSARIFResult converts one diagnostic to a SARIF result.
func buildSARIFResult(d *lint.Diagnostic, ruleIndex map[string]int) sarifResult21 {
	return sarifResult21{
		RuleID:    d.RuleID,
		RuleIndex: ruleIndex[d.RuleID],
		Level:     sarifLevelFor(d.Severity),
		Message:   sarifText21{Text: d.Message},
		Locations: []sarifLocation21{buildSARIFLocation(d)},
	}
}

// buildSARIFLocation constructs a physical location from a diagnostic.
// The region is always emitted — DisplayLine clamps line 0 to 1, which
// is valid SARIF for a file-level issue. StartColumn is omitted when
// the column is unknown (zero).
func buildSARIFLocation(d *lint.Diagnostic) sarifLocation21 {
	region := &sarifRegion21{StartLine: d.DisplayLine()}
	if d.Column > 0 {
		region.StartColumn = d.Column
	}
	return sarifLocation21{PhysicalLocation: sarifPhysical21{
		ArtifactLocation: sarifArtifact21{URI: d.File},
		Region:           region,
	}}
}

// -- SARIF 2.1.0 structs --------------------------------------------------
// Field order matches the SARIF 2.1.0 schema for readable output.

type sarifDoc21 struct {
	Schema  string       `json:"$schema"`
	Version string       `json:"version"`
	Runs    []sarifRun21 `json:"runs"`
}

type sarifRun21 struct {
	Tool    sarifTool21     `json:"tool"`
	Results []sarifResult21 `json:"results"`
}

type sarifTool21 struct {
	Driver sarifDriver21 `json:"driver"`
}

type sarifDriver21 struct {
	Name           string        `json:"name"`
	Version        string        `json:"version,omitempty"`
	InformationURI string        `json:"informationUri"`
	Rules          []sarifRule21 `json:"rules"`
}

type sarifRule21 struct {
	ID               string      `json:"id"`
	HelpURI          string      `json:"helpUri,omitempty"`
	ShortDescription sarifText21 `json:"shortDescription"`
}

type sarifText21 struct {
	Text string `json:"text"`
}

type sarifResult21 struct {
	RuleID    string            `json:"ruleId"`
	RuleIndex int               `json:"ruleIndex"`
	Level     string            `json:"level"`
	Message   sarifText21       `json:"message"`
	Locations []sarifLocation21 `json:"locations"`
}

type sarifLocation21 struct {
	PhysicalLocation sarifPhysical21 `json:"physicalLocation"`
}

type sarifPhysical21 struct {
	ArtifactLocation sarifArtifact21 `json:"artifactLocation"`
	Region           *sarifRegion21  `json:"region,omitempty"`
}

type sarifArtifact21 struct {
	URI string `json:"uri"`
}

type sarifRegion21 struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
}
