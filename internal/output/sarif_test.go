package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSARIFFormatter_ImplementsFormatter(t *testing.T) {
	var _ Formatter = &SARIFFormatter{}
}

func TestSARIFFormatter_ValidSARIF21(t *testing.T) {
	f := &SARIFFormatter{ToolVersion: "v1.0.0"}
	var buf bytes.Buffer
	diags := []lint.Diagnostic{{
		File:     "README.md",
		Line:     10,
		Column:   5,
		RuleID:   "MDS001",
		RuleName: "line-length",
		Severity: lint.Error,
		Message:  "line too long",
	}}

	require.NoError(t, f.Format(&buf, diags))

	var top map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &top))
	assert.Equal(t, "2.1.0", top["version"])
	assert.Equal(t, "https://json.schemastore.org/sarif-2.1.0.json", top["$schema"])
}

func TestSARIFFormatter_SingleDiagnosticShape(t *testing.T) {
	f := &SARIFFormatter{ToolVersion: "v1.2.3"}
	var buf bytes.Buffer
	diags := []lint.Diagnostic{{
		File:     "README.md",
		Line:     10,
		Column:   5,
		RuleID:   "MDS001",
		RuleName: "line-length",
		Severity: lint.Error,
		Message:  "line too long",
	}}

	require.NoError(t, f.Format(&buf, diags))

	var doc sarifDoc21
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))

	require.Len(t, doc.Runs, 1)
	drv := doc.Runs[0].Tool.Driver
	assert.Equal(t, "mdsmith", drv.Name)
	assert.Equal(t, "v1.2.3", drv.Version)
	assert.Equal(t, "https://mdsmith.dev", drv.InformationURI)
	require.Len(t, drv.Rules, 1)
	assert.Equal(t, "MDS001", drv.Rules[0].ID)
	assert.Equal(t, "https://mdsmith.dev/rules/mds001-line-length/", drv.Rules[0].HelpURI)
	assert.Equal(t, "line-length", drv.Rules[0].ShortDescription.Text)

	require.Len(t, doc.Runs[0].Results, 1)
	r := doc.Runs[0].Results[0]
	assert.Equal(t, "MDS001", r.RuleID)
	assert.Equal(t, 0, r.RuleIndex)
	assert.Equal(t, "error", r.Level)
	assert.Equal(t, "line too long", r.Message.Text)
	require.Len(t, r.Locations, 1)
	phys := r.Locations[0].PhysicalLocation
	assert.Equal(t, "README.md", phys.ArtifactLocation.URI)
	require.NotNil(t, phys.Region)
	assert.Equal(t, 10, phys.Region.StartLine)
	assert.Equal(t, 5, phys.Region.StartColumn)
}

func TestSARIFFormatter_EmptyDiagnostics(t *testing.T) {
	f := &SARIFFormatter{}
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, []lint.Diagnostic{}))
	var doc sarifDoc21
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.Len(t, doc.Runs, 1)
	assert.Empty(t, doc.Runs[0].Results)
	assert.Empty(t, doc.Runs[0].Tool.Driver.Rules)
}

func TestSARIFFormatter_SeverityMapping(t *testing.T) {
	f := &SARIFFormatter{}
	diags := []lint.Diagnostic{
		{File: "a.md", Line: 1, RuleID: "MDS001", RuleName: "r1", Severity: lint.Error, Message: "e"},
		{File: "b.md", Line: 2, RuleID: "MDS002", RuleName: "r2", Severity: lint.Warning, Message: "w"},
	}
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, diags))
	var doc sarifDoc21
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.Len(t, doc.Runs[0].Results, 2)
	assert.Equal(t, "error", doc.Runs[0].Results[0].Level)
	assert.Equal(t, "warning", doc.Runs[0].Results[1].Level)
}

func TestSARIFFormatter_DeduplicatesRules(t *testing.T) {
	f := &SARIFFormatter{}
	diags := []lint.Diagnostic{
		{File: "a.md", Line: 1, RuleID: "MDS001", RuleName: "line-length", Severity: lint.Error, Message: "m"},
		{File: "b.md", Line: 2, RuleID: "MDS001", RuleName: "line-length", Severity: lint.Error, Message: "m"},
	}
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, diags))
	var doc sarifDoc21
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.Len(t, doc.Runs[0].Tool.Driver.Rules, 1, "same rule must appear once in driver.rules")
	require.Len(t, doc.Runs[0].Results, 2)
	assert.Equal(t, 0, doc.Runs[0].Results[0].RuleIndex)
	assert.Equal(t, 0, doc.Runs[0].Results[1].RuleIndex)
}

func TestSARIFFormatter_MultipleRulesIndexed(t *testing.T) {
	f := &SARIFFormatter{}
	diags := []lint.Diagnostic{
		{File: "a.md", Line: 1, RuleID: "MDS001", RuleName: "line-length", Severity: lint.Error, Message: "m"},
		{File: "b.md", Line: 2, RuleID: "MDS002", RuleName: "first-heading", Severity: lint.Warning, Message: "w"},
		{File: "c.md", Line: 3, RuleID: "MDS001", RuleName: "line-length", Severity: lint.Error, Message: "m"},
	}
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, diags))
	var doc sarifDoc21
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.Len(t, doc.Runs[0].Tool.Driver.Rules, 2)
	require.Len(t, doc.Runs[0].Results, 3)
	assert.Equal(t, 0, doc.Runs[0].Results[0].RuleIndex) // MDS001 → index 0
	assert.Equal(t, 1, doc.Runs[0].Results[1].RuleIndex) // MDS002 → index 1
	assert.Equal(t, 0, doc.Runs[0].Results[2].RuleIndex) // MDS001 → index 0
}

func TestSARIFFormatter_ColumnOmittedWhenZero(t *testing.T) {
	f := &SARIFFormatter{}
	diags := []lint.Diagnostic{
		{File: "a.md", Line: 5, Column: 0, RuleID: "MDS001", RuleName: "r", Severity: lint.Error, Message: "m"},
	}
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, diags))

	// Unmarshal raw to check startColumn absence.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &raw))
	runs := raw["runs"].([]any)
	results := runs[0].(map[string]any)["results"].([]any)
	locs := results[0].(map[string]any)["locations"].([]any)
	region := locs[0].(map[string]any)["physicalLocation"].(map[string]any)["region"].(map[string]any)
	_, hasCol := region["startColumn"]
	assert.False(t, hasCol, "startColumn should be omitted when column is zero")
	assert.EqualValues(t, 5, region["startLine"])
}

func TestSARIFFormatter_ZeroLineClampsToOne(t *testing.T) {
	f := &SARIFFormatter{}
	diags := []lint.Diagnostic{
		{File: "a.md", Line: 0, RuleID: "MDS001", RuleName: "r", Severity: lint.Error, Message: "m"},
	}
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, diags))
	var doc sarifDoc21
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.NotNil(t, doc.Runs[0].Results[0].Locations[0].PhysicalLocation.Region)
	assert.Equal(t, 1, doc.Runs[0].Results[0].Locations[0].PhysicalLocation.Region.StartLine,
		"DisplayLine clamps line 0 to 1")
}

func TestSARIFFormatter_VersionOmittedWhenEmpty(t *testing.T) {
	f := &SARIFFormatter{} // no ToolVersion
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, []lint.Diagnostic{}))
	var raw map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &raw))
	runs := raw["runs"].([]any)
	driver := runs[0].(map[string]any)["tool"].(map[string]any)["driver"].(map[string]any)
	_, hasVer := driver["version"]
	assert.False(t, hasVer, "version should be omitted when ToolVersion is empty")
}

// -- sarifHelpURI ---------------------------------------------------------

func TestSarifHelpURI_WithName(t *testing.T) {
	d := &lint.Diagnostic{RuleID: "MDS001", RuleName: "line-length"}
	assert.Equal(t, "https://mdsmith.dev/rules/mds001-line-length/", sarifHelpURI(d))
}

func TestSarifHelpURI_EmptyID(t *testing.T) {
	d := &lint.Diagnostic{RuleID: "", RuleName: "name"}
	assert.Equal(t, "", sarifHelpURI(d))
}

func TestSarifHelpURI_NoName(t *testing.T) {
	d := &lint.Diagnostic{RuleID: "MDS999"}
	assert.Equal(t, "https://mdsmith.dev/rules/mds999/", sarifHelpURI(d))
}

// -- sarifLevelFor --------------------------------------------------------

func TestSarifLevelFor_Error(t *testing.T) {
	assert.Equal(t, "error", sarifLevelFor(lint.Error))
}

func TestSarifLevelFor_Warning(t *testing.T) {
	assert.Equal(t, "warning", sarifLevelFor(lint.Warning))
}

func TestSarifLevelFor_Unknown(t *testing.T) {
	assert.Equal(t, "note", sarifLevelFor(lint.Severity("unknown")))
}
