package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSchemaWithCrossRefs(refs ...CrossRef) *Schema {
	return &Schema{Source: "test", RootLevel: 2, CrossReferences: refs}
}

func TestCrossRefs_UnresolvedFlagged(t *testing.T) {
	src := "# Doc\n\nFollow Step 7 to continue.\n\n## Step 1\n"
	f := newDocFile(t, "doc.md", src)
	sch := newSchemaWithCrossRefs(CrossRef{
		Pattern:   `\bStep (\d+)\b`,
		MustMatch: "Step {n}",
	})
	diags := ValidateCrossReferences(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Contains(t, diags[0].Message, "Step 7")
}

func TestCrossRefs_ResolvedPasses(t *testing.T) {
	src := "# Doc\n\nSee Step 1 for the procedure.\n\n## Step 1\n"
	f := newDocFile(t, "doc.md", src)
	sch := newSchemaWithCrossRefs(CrossRef{
		Pattern:   `\bStep (\d+)\b`,
		MustMatch: "Step {n}",
	})
	diags := ValidateCrossReferences(f, sch, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestCrossRefs_SkipBlockquote(t *testing.T) {
	src := "# Doc\n\n> Step 99 was removed.\n\nSee Step 1.\n\n## Step 1\n"
	f := newDocFile(t, "doc.md", src)
	sch := newSchemaWithCrossRefs(CrossRef{
		Pattern:           `\bStep (\d+)\b`,
		MustMatch:         "Step {n}",
		SkipLinesMatching: "^> ",
	})
	diags := ValidateCrossReferences(f, sch, makeDiagForTest)
	assert.Empty(t, diags, "blockquoted Step 99 should be skipped")
}

func TestAcronyms_FirstUseFlagged(t *testing.T) {
	src := "# Doc\n\nOIDC handles login.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Acronyms: &AcronymRule{
		KnownSafe: []string{"API"},
	}}
	diags := ValidateAcronyms(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "OIDC")
}

func TestAcronyms_KnownSafePasses(t *testing.T) {
	src := "# Doc\n\nHTTP and API are the basics.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Acronyms: &AcronymRule{
		KnownSafe: []string{"API", "HTTP"},
	}}
	diags := ValidateAcronyms(f, sch, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestAcronyms_ExpansionPasses(t *testing.T) {
	src := "# Doc\n\nOIDC (OpenID Connect) is configured.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Acronyms: &AcronymRule{}}
	diags := ValidateAcronyms(f, sch, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestAcronyms_ScopedOnlyFiresInScope(t *testing.T) {
	src := `# Doc

## Check

OIDC needs an expansion here.

## Notes

OIDC outside scope — should not flag.
`
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		Sections: []Scope{
			{Heading: "Check", Required: true},
			{Heading: "Notes", Required: false},
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, makeDiagForTest)
	require.Len(t, diags, 1, "exactly one diagnostic, inside Check")
	assert.Contains(t, diags[0].Message, "OIDC")
	assert.Equal(t, 5, diags[0].Line)
}

func TestIndex_HeadingsShape(t *testing.T) {
	src := "# Title\n\n## Goal\n\n## Tasks\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	data, err := BuildIndex(f, sch)
	require.NoError(t, err)
	var got map[string][]IndexHeading
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Len(t, got[IndexIncludeHeadingsFlat], 3)
	assert.Equal(t, "title", got[IndexIncludeHeadingsFlat][0].Slug)
	assert.Equal(t, "goal", got[IndexIncludeHeadingsFlat][1].Slug)
	assert.Equal(t, 1, got[IndexIncludeHeadingsFlat][0].Level)
	assert.Equal(t, 2, got[IndexIncludeHeadingsFlat][1].Level)
}

func TestIndex_StepMapShape(t *testing.T) {
	src := "# Title\n\n## Section\n\n### Step 1\n\n### Step 2\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeStepMap},
	}}
	data, err := BuildIndex(f, sch)
	require.NoError(t, err)
	var got map[string]map[string][]string
	require.NoError(t, json.Unmarshal(data, &got))
	stepMap := got[IndexIncludeStepMap]
	assert.Equal(t, []string{"section"}, stepMap["title"])
	assert.Equal(t, []string{"step-1", "step-2"}, stepMap["section"])
}

func TestWriteIndex_WritesNextToSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# Title\n\n## Goal\n"), 0o644))
	f, err := lint.NewFile(path, []byte("# Title\n\n## Goal\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "doc-index.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	require.NoError(t, WriteIndex(f, sch))
	data, err := os.ReadFile(filepath.Join(dir, "doc-index.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"slug": "title"`)
}

func TestWriteIndex_RejectsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "/etc/hosts",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	err = WriteIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be relative")
}

func TestWriteIndex_RejectsWindowsAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)

	cases := []string{`C:\out.json`, `c:/out.json`, `\out.json`}
	for _, out := range cases {
		t.Run(out, func(t *testing.T) {
			sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
				Output:  out,
				Include: []string{IndexIncludeHeadingsFlat},
			}}
			err = WriteIndex(f, sch)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must be relative")
		})
	}
}

func TestWriteIndex_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# Title\n"), 0o644))
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  ".mdsmith/index/runbook.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	require.NoError(t, WriteIndex(f, sch))
	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith/index/runbook.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"slug": "title"`)
}

func TestValidateIndex_MissingReportsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "absent.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "missing")
}

func TestValidateIndex_StaleReportsOutOfDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "stale.json"), []byte("{}\n"), 0o644))
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "stale.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "out of date")
}

func TestValidateIndex_ReadErrorSurfacesDistinctMessage(t *testing.T) {
	// A directory at the index path triggers a read error that is
	// not os.IsNotExist; the validator should surface the read
	// error verbatim instead of misreporting it as "missing".
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "blocked.json"), 0o755))
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "blocked.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "cannot be read")
}

func TestWriteIndex_RejectsParentTraversal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "../escape.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	err = WriteIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "..")
}

func TestParseInline_CrossReferencesAndAcronymsAndIndex(t *testing.T) {
	raw := map[string]any{
		"cross-references": []any{
			map[string]any{
				"pattern":             `\bStep (\d+)\b`,
				"must-match":          "Step {n}",
				"skip-lines-matching": "^> ",
			},
		},
		"acronyms": map[string]any{
			"known-safe": []any{"API", "HTTP"},
			"scope":      []any{"Check"},
		},
		"index": map[string]any{
			"output":  ".runbook-index.json",
			"include": []any{"step-map", "headings"},
		},
	}
	sch, err := ParseInline(raw, "test")
	require.NoError(t, err)
	require.Len(t, sch.CrossReferences, 1)
	assert.Equal(t, "Step {n}", sch.CrossReferences[0].MustMatch)
	require.NotNil(t, sch.Acronyms)
	assert.Equal(t, []string{"Check"}, sch.Acronyms.Scope)
	require.NotNil(t, sch.Index)
	assert.Equal(t, ".runbook-index.json", sch.Index.Output)
	assert.Equal(t, []string{"step-map", "headings"}, sch.Index.Include)
}

func TestParseInline_IndexUnknownIncludeRejected(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"index": map[string]any{
			"output":  "x.json",
			"include": []any{"bogus"},
		},
	}, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestIndex_WordCountsAndCrossRefGraphShape(t *testing.T) {
	src := "# Title\n\nIntro paragraph here.\n\n## Step 1\n\nSee Step 2 for details.\n\n## Step 2\n\nMore text.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2,
		CrossReferences: []CrossRef{{
			Pattern:   `\bStep (\d+)\b`,
			MustMatch: "Step {n}",
		}},
		Index: &IndexSpec{
			Output: "out.json",
			Include: []string{
				IndexIncludeWordCounts,
				IndexIncludeCrossRefs,
			},
		},
	}
	data, err := BuildIndex(f, sch)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))

	wc, ok := got[IndexIncludeWordCounts].(map[string]any)
	require.True(t, ok, "word-counts must be an object")
	// Title body is "Intro paragraph here." (3 words) — sub-section
	// "Step 1" body should be its own count, not the parent's.
	assert.EqualValues(t, 3, wc["title"])
	assert.EqualValues(t, 5, wc["step-1"])
	assert.EqualValues(t, 2, wc["step-2"])

	graph, ok := got[IndexIncludeCrossRefs].(map[string]any)
	require.True(t, ok, "cross-ref-graph must be an object")
	assert.Equal(t, "step-2", graph["Step 2"])
}

func TestFillTemplate_NumericAndNamedCaptures(t *testing.T) {
	re := regexp.MustCompile(`Step (?P<num>\d+)`)
	match := re.FindStringSubmatch("Step 42")
	got, err := fillTemplate("Step {1}", match, re.SubexpNames())
	require.NoError(t, err)
	assert.Equal(t, "Step 42", got)

	got, err = fillTemplate("Step {num}", match, re.SubexpNames())
	require.NoError(t, err)
	assert.Equal(t, "Step 42", got)

	_, err = fillTemplate("Step {9}", match, re.SubexpNames())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	_, err = fillTemplate("Step {bogus}", match, re.SubexpNames())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown placeholder")

	_, err = fillTemplate("Step {", match, re.SubexpNames())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unterminated")

	_, err = fillTemplate("Step {}", match, re.SubexpNames())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty placeholder")
}

func TestParseInline_CrossRefMissingPatternRejected(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"cross-references": []any{
			map[string]any{"must-match": "Step {n}"},
		},
	}, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern")
}
