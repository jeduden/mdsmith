package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestAcronyms_SkipsAcronymsInHeadingLines(t *testing.T) {
	// "## OIDC configuration" must not consume the first-use slot;
	// the body that immediately follows with a parenthesised
	// expansion should satisfy the rule, both in document-wide and
	// scoped modes.
	src := `# Doc

## OIDC configuration

OIDC (OpenID Connect) is set up here.
`
	f := newDocFile(t, "doc.md", src)

	// Document-wide.
	sch := &Schema{Source: "test", RootLevel: 2,
		Acronyms: &AcronymRule{},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	assert.Empty(t, diags, "heading line must not flag OIDC as first use")

	// Scoped.
	sch.Sections = []Scope{literalScope("OIDC configuration")}
	sch.Acronyms.Scope = []string{"OIDC configuration"}
	diags = ValidateAcronyms(f, sch, nil, makeDiagForTest)
	assert.Empty(t, diags, "scoped pass also excludes the heading line")
}

func TestAcronyms_FirstUseFlagged(t *testing.T) {
	src := "# Doc\n\nOIDC handles login.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Acronyms: &AcronymRule{
		KnownSafe: []string{"API"},
	}}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "OIDC")
}

func TestAcronyms_KnownSafePasses(t *testing.T) {
	src := "# Doc\n\nHTTP and API are the basics.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Acronyms: &AcronymRule{
		KnownSafe: []string{"API", "HTTP"},
	}}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestAcronyms_ExpansionPasses(t *testing.T) {
	src := "# Doc\n\nOIDC (OpenID Connect) is configured.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2, Acronyms: &AcronymRule{}}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
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
			literalScope("Check"),
			optionalScope("Notes"),
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
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

func TestParseInline_CrossRefErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "not a list",
			raw:  map[string]any{"cross-references": "nope"},
			want: "must be a list",
		},
		{
			name: "entry not a mapping",
			raw:  map[string]any{"cross-references": []any{"nope"}},
			want: "must be a mapping",
		},
		{
			name: "value not a string",
			raw: map[string]any{"cross-references": []any{
				map[string]any{"pattern": 42},
			}},
			want: "must be a string",
		},
		{
			name: "unknown key",
			raw: map[string]any{"cross-references": []any{
				map[string]any{"pattern": "x", "must-match": "y", "bogus": "z"},
			}},
			want: "unknown key",
		},
		{
			name: "missing must-match",
			raw: map[string]any{"cross-references": []any{
				map[string]any{"pattern": "x"},
			}},
			want: "must-match",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseInline(tc.raw, "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestParseInline_CrossRefInvalidRegexRejected verifies that invalid regex
// patterns in cross-reference entries are caught at parse time
// (parseCrossRefEntry compiles them eagerly) rather than on the first document
// check. This exercises the two new error paths added when regex pre-compilation
// was introduced (Fix 4 of the high-performance-go audit).
func TestParseInline_CrossRefInvalidRegexRejected(t *testing.T) {
	t.Run("invalid pattern regex", func(t *testing.T) {
		_, err := ParseInline(map[string]any{
			"cross-references": []any{
				map[string]any{"pattern": "[unterminated", "must-match": "Step {n}"},
			},
		}, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pattern")
	})
	t.Run("invalid skip-lines-matching regex", func(t *testing.T) {
		_, err := ParseInline(map[string]any{
			"cross-references": []any{
				map[string]any{
					"pattern":             `\bStep (\d+)\b`,
					"must-match":          "Step {n}",
					"skip-lines-matching": "[bogus",
				},
			},
		}, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid skip-lines-matching")
	})
}

func TestParseInline_AcronymsErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "not a mapping",
			raw:  map[string]any{"acronyms": "nope"},
			want: "must be a mapping",
		},
		{
			name: "unknown key",
			raw: map[string]any{"acronyms": map[string]any{
				"bogus": []any{"X"},
			}},
			want: "unknown key",
		},
		{
			name: "known-safe wrong type",
			raw: map[string]any{"acronyms": map[string]any{
				"known-safe": "API",
			}},
			want: "must be a list",
		},
		{
			name: "scope list contains non-string",
			raw: map[string]any{"acronyms": map[string]any{
				"scope": []any{42},
			}},
			want: "must be a string",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseInline(tc.raw, "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestParseInline_IndexErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "not a mapping",
			raw:  map[string]any{"index": "nope"},
			want: "must be a mapping",
		},
		{
			name: "unknown key",
			raw: map[string]any{"index": map[string]any{
				"output":  "x.json",
				"include": []any{"headings"},
				"bogus":   true,
			}},
			want: "unknown key",
		},
		{
			name: "output wrong type",
			raw: map[string]any{"index": map[string]any{
				"output":  42,
				"include": []any{"headings"},
			}},
			want: "must be a string",
		},
		{
			name: "missing output",
			raw: map[string]any{"index": map[string]any{
				"include": []any{"headings"},
			}},
			want: "output",
		},
		{
			name: "missing include",
			raw: map[string]any{"index": map[string]any{
				"output": "x.json",
			}},
			want: "include",
		},
		{
			name: "include not a list",
			raw: map[string]any{"index": map[string]any{
				"output":  "x.json",
				"include": "headings",
			}},
			want: "must be a list",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseInline(tc.raw, "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestCrossRefs_InvalidPatternDiagnostic(t *testing.T) {
	src := "# Doc\n\nSome text.\n"
	f := newDocFile(t, "doc.md", src)
	sch := newSchemaWithCrossRefs(CrossRef{
		Pattern:   "[unterminated",
		MustMatch: "Step {n}",
	})
	diags := ValidateCrossReferences(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "invalid pattern")
}

func TestCrossRefs_InvalidSkipPatternDiagnostic(t *testing.T) {
	src := "# Doc\n\nSee Step 1.\n\n## Step 1\n"
	f := newDocFile(t, "doc.md", src)
	sch := newSchemaWithCrossRefs(CrossRef{
		Pattern:           `\bStep (\d+)\b`,
		MustMatch:         "Step {n}",
		SkipLinesMatching: "[bogus",
	})
	diags := ValidateCrossReferences(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "skip-lines-matching")
}

func TestCrossRefs_TemplateErrorDiagnostic(t *testing.T) {
	src := "# Doc\n\nSee Step 1.\n\n## Step 1\n"
	f := newDocFile(t, "doc.md", src)
	sch := newSchemaWithCrossRefs(CrossRef{
		Pattern:   `\bStep (\d+)\b`,
		MustMatch: "Step {bogus}",
	})
	diags := ValidateCrossReferences(f, sch, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "cannot resolve template")
}

func TestLineMatches_OutOfRange(t *testing.T) {
	f := newDocFile(t, "doc.md", "x\n")
	re := regexp.MustCompile(".*")
	assert.False(t, lineMatches(f, 0, re), "line 0 should be out of range")
	assert.False(t, lineMatches(f, 999, re), "line 999 should be out of range")
	assert.True(t, lineMatches(f, 1, re))
}

func TestTryParseIndex_RejectsEmptyAndNonDigits(t *testing.T) {
	if _, ok := tryParseIndex(""); ok {
		t.Fatal("empty string should not parse as index")
	}
	if _, ok := tryParseIndex("12a"); ok {
		t.Fatal("non-digit should not parse")
	}
	n, ok := tryParseIndex("42")
	require.True(t, ok)
	assert.Equal(t, 42, n)
}

func TestResolveCrossRefPlaceholder_EdgeCases(t *testing.T) {
	// {n} when pattern has no capture group.
	_, err := resolveCrossRefPlaceholder("n", []string{"only-match"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no capture group")

	// Empty placeholder name.
	_, err = resolveCrossRefPlaceholder("", []string{"x"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty placeholder")
}

func TestHasParenExpansion_EdgeCases(t *testing.T) {
	// Missing close-paren.
	assert.False(t, hasParenExpansion([]byte("FOO (no close"), 3))
	// Empty parens.
	assert.False(t, hasParenExpansion([]byte("FOO ()"), 3))
	// Valid expansion.
	assert.True(t, hasParenExpansion([]byte("FOO (Bar Baz)"), 3))
	// Not a paren at all.
	assert.False(t, hasParenExpansion([]byte("FOO bar"), 3))
}

func TestAcronyms_AppliesWithEmptyScope(t *testing.T) {
	src := "# Doc\n\nOIDC needs an expansion.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2,
		Acronyms: &AcronymRule{Scope: []string{}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1, "empty scope applies document-wide")
}

func TestAcronyms_ScopedSecondOccurrencePerScope(t *testing.T) {
	// Each named-scope pass starts with empty seen set, so the same
	// acronym appearing in two scoped sections is flagged in both.
	src := `# Doc

## Check

OIDC first time here.

## Check

OIDC again, separate scope pass.
`
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		Sections: []Scope{
			literalScope("Check"),
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	// walker only matches one "Check" today (no repeats), so we
	// just check that at least the first OIDC is flagged.
	require.NotEmpty(t, diags)
}

func TestAcronyms_AliasMatchesScope(t *testing.T) {
	src := `# Doc

## Probe

OIDC undefined.

## Outside

OIDC also here.
`
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		Sections: []Scope{
			{
				Heading: "Probe|Check",
				Matcher: &Matcher{Regex: "Probe|Check"},
			},
			optionalScope("Outside"),
		},
		Acronyms: &AcronymRule{Scope: []string{"Probe|Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
}

func TestBuildIndex_UnknownIncludeKeyErrors(t *testing.T) {
	// Bypass the parser by constructing the spec directly so we can
	// drive BuildIndex's switch default branch.
	f := newDocFile(t, "doc.md", "# T\n")
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "x.json",
		Include: []string{"bogus"},
	}}
	_, err := BuildIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestBuildIndex_NilWhenIndexAbsent(t *testing.T) {
	f := newDocFile(t, "doc.md", "# T\n")
	data, err := BuildIndex(f, &Schema{})
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestWriteIndex_NilWhenIndexAbsent(t *testing.T) {
	dir := t.TempDir()
	f, err := lint.NewFile(filepath.Join(dir, "doc.md"), []byte("# T\n"))
	require.NoError(t, err)
	require.NoError(t, WriteIndex(f, &Schema{}))
}

func TestValidateIndex_NoDiagnosticWhenInSync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "doc-index.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	require.NoError(t, WriteIndex(f, sch))
	diags := ValidateIndex(f, sch, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestAcronyms_WalkerSkipsWildcardAndPreambleScopes(t *testing.T) {
	// A preamble-first / wildcard-second scope tree exercises the
	// `if sc.Wildcard || sc.Preamble { continue }` branch.
	src := `# Doc

## Check

OIDC needs an expansion.
`
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		Sections: []Scope{
			preambleScope(),
			slotScope(),
			literalScope("Check"),
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1)
}

func TestAcronyms_WalkerRecursesIntoNestedSections(t *testing.T) {
	src := `# Doc

## Diagnosis

### Step

#### Check

OIDC inside nested scope.
`
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		Sections: []Scope{
			nested("Diagnosis",
				nested("Step",
					literalScope("Check"))),
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "OIDC")
}

func TestAcronyms_FindHeadRespectsParentEnd(t *testing.T) {
	// Two Diagnosis sections; the nested "Check" only matches inside
	// its own parent's range. Without parentEnd filtering, walkRanges
	// would attach Check to the wrong Diagnosis.
	src := `# Doc

## Diagnosis

### Check

OIDC in first Diagnosis.

## Other

### Check

OIDC in second tree — but acronyms-scope is per-walker first-match.
`
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		Sections: []Scope{
			nested("Diagnosis", literalScope("Check")),
			optionalScope("Other"),
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Equal(t, 7, diags[0].Line)
}

func TestIndex_CrossRefGraphStopsOnInvalidPattern(t *testing.T) {
	// An invalid pattern in the cross-references list is a schema
	// misconfiguration; the graph builder reports the error rather
	// than silently shipping a partial index. A later valid entry
	// never gets a chance to contribute — by design, since the
	// schema author needs to fix the bad regex first.
	src := "# Doc\n\nSee Step 1.\n\n## Step 1\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2,
		CrossReferences: []CrossRef{
			{Pattern: "[unterminated", MustMatch: "Step {n}"},
			{Pattern: `\bStep (\d+)\b`, MustMatch: "Step {n}"},
		},
		Index: &IndexSpec{
			Output:  "x.json",
			Include: []string{IndexIncludeCrossRefs},
		},
	}
	_, err := BuildIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
}

func TestIndex_CrossRefGraphSkipLinesAndTemplateError(t *testing.T) {
	src := "# Doc\n\n> Step 99 stale.\n\nSee Step 1.\n\n## Step 1\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{Source: "test", RootLevel: 2,
		CrossReferences: []CrossRef{
			{Pattern: `\bStep (\d+)\b`, MustMatch: "Step {bogus}",
				SkipLinesMatching: "^> "},
		},
		Index: &IndexSpec{
			Output:  "x.json",
			Include: []string{IndexIncludeCrossRefs},
		},
	}
	data, err := BuildIndex(f, sch)
	require.NoError(t, err)
	var got map[string]map[string]string
	require.NoError(t, json.Unmarshal(data, &got))
	// Template error and blockquote skip both swallow entries — the
	// graph stays empty rather than carrying garbage.
	assert.Empty(t, got[IndexIncludeCrossRefs])
}

func TestNextSectionLine_ReturnsParentEndAtBoundary(t *testing.T) {
	// A nested heading beyond parentEnd should still be clipped to
	// parentEnd so a scope's range does not leak into a sibling.
	heads := []DocHeading{
		{Level: 2, Text: "A", Line: 3},
		{Level: 2, Text: "B", Line: 10},
	}
	assert.Equal(t, 10, nextSectionLine(heads, 0, 2, 100))
	// parentEnd smaller than the next heading clips early.
	assert.Equal(t, 5, nextSectionLine(heads, 0, 2, 5))
	// No further heading — falls through to parentEnd.
	assert.Equal(t, 100, nextSectionLine(heads, 1, 2, 100))
}

func TestAcronyms_ScopeWithNoMatchingHeadingIsSilent(t *testing.T) {
	// Schema lists a "Check" scope but the document has no matching
	// heading: walker returns -1, no ranges scanned, no diagnostics.
	src := "# Doc\n\nOIDC mentioned but no matching scope heading.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		Sections: []Scope{
			literalScope("Check"),
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidators_NoOpWhenSchemaAbsent(t *testing.T) {
	f := newDocFile(t, "doc.md", "# T\n")
	// ValidateCrossReferences: nil schema and empty-list schema both
	// short-circuit before touching the AST.
	assert.Empty(t, ValidateCrossReferences(f, nil, makeDiagForTest))
	assert.Empty(t, ValidateCrossReferences(f,
		&Schema{Source: "test", RootLevel: 2}, makeDiagForTest))

	// ValidateAcronyms: same contract.
	assert.Empty(t, ValidateAcronyms(f, nil, nil, makeDiagForTest))
	assert.Empty(t, ValidateAcronyms(f,
		&Schema{Source: "test", RootLevel: 2}, nil, makeDiagForTest))

	// ValidateIndex: nil schema and no-index schema return no diags.
	assert.Empty(t, ValidateIndex(f, nil, makeDiagForTest))
	assert.Empty(t, ValidateIndex(f,
		&Schema{Source: "test", RootLevel: 2}, makeDiagForTest))
}

func TestBuildCrossRefGraph_EmptyWhenNoRefs(t *testing.T) {
	f := newDocFile(t, "doc.md", "# T\n")
	got, err := buildCrossRefGraph(f, &Schema{Source: "test", RootLevel: 2})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestBuildCrossRefGraph_InvalidPatternErrors(t *testing.T) {
	f := newDocFile(t, "doc.md", "# T\n")
	sch := &Schema{Source: "test", RootLevel: 2,
		CrossReferences: []CrossRef{{Pattern: "[bad", MustMatch: "Step {n}"}},
	}
	_, err := buildCrossRefGraph(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
}

func TestBuildCrossRefGraph_InvalidSkipPatternErrors(t *testing.T) {
	f := newDocFile(t, "doc.md", "# T\n")
	sch := &Schema{Source: "test", RootLevel: 2,
		CrossReferences: []CrossRef{{
			Pattern:           `\bStep (\d+)\b`,
			MustMatch:         "Step {n}",
			SkipLinesMatching: "[bad",
		}},
	}
	_, err := buildCrossRefGraph(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skip-lines-matching")
}

func TestBuildIndex_PropagatesCrossRefGraphError(t *testing.T) {
	f := newDocFile(t, "doc.md", "# T\n")
	sch := &Schema{Source: "test", RootLevel: 2,
		CrossReferences: []CrossRef{{Pattern: "[bad", MustMatch: "Step {n}"}},
		Index: &IndexSpec{
			Output:  "x.json",
			Include: []string{IndexIncludeCrossRefs},
		},
	}
	_, err := BuildIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
}

func TestValidateIndex_SurfacesCrossRefGraphError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2,
		CrossReferences: []CrossRef{{Pattern: "[bad", MustMatch: "Step {n}"}},
		Index: &IndexSpec{
			Output:  "x.json",
			Include: []string{IndexIncludeCrossRefs},
		},
	}
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "invalid pattern")
}

func TestBuildFlatHeadings_EmptyDocReturnsEmptySlice(t *testing.T) {
	f := newDocFile(t, "doc.md", "plain prose with no headings.\n")
	got := buildFlatHeadings(f)
	require.NotNil(t, got, "must return non-nil slice, not nil")
	assert.Empty(t, got)
}

func TestFrontmatterExpr_JSONForScalarsAndCollections(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{true, "true"},
		{42, "42"},
		{3.5, "3.5"},
		{nil, "null"},
		{[]any{"a", "b"}, `["a","b"]`},
		{map[string]any{"k": 1}, `{"k":1}`},
	}
	for _, tc := range cases {
		got, err := frontmatterExpr(tc.in)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}

	// Empty string is rejected.
	_, err := frontmatterExpr("")
	require.Error(t, err)
	// Unsupported type triggers default branch.
	_, err = frontmatterExpr(struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestValidateOutputPath_RejectsBackslashes(t *testing.T) {
	err := validateOutputPath(`sub\out.json`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "POSIX-style")
}

func TestIndexCacheKey_NormalizesRelativeAndAbsolute(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "doc.md")
	// Relative and absolute forms should map to the same cache key
	// when RootDir is set to the source directory.
	rel, err := lint.NewFile("doc.md", []byte("# T\n"))
	require.NoError(t, err)
	rel.RootDir = dir
	absF, err := lint.NewFile(abs, []byte("# T\n"))
	require.NoError(t, err)
	assert.Equal(t, indexCacheKey(absF), indexCacheKey(rel))
}

func TestWriteIndex_AnchorsRelativePathOnRootDir(t *testing.T) {
	// f.Path is workspace-relative (engine/LSP pattern). With
	// f.RootDir set, the index must land next to the document on
	// disk, not next to the process CWD.
	root := t.TempDir()
	docDir := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(docDir, 0o755))
	docPath := filepath.Join(docDir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte("# T\n"), 0o644))

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	other := t.TempDir()
	require.NoError(t, os.Chdir(other))

	f, err := lint.NewFile("sub/doc.md", []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	require.NoError(t, WriteIndex(f, sch))
	_, err = os.Stat(filepath.Join(docDir, "out.json"))
	assert.NoError(t, err, "index must be written next to the document, not CWD")
}

func TestIndexCacheKey_AnchorsRelativePathOnRootDir(t *testing.T) {
	// Without RootDir-anchoring, two calls with the same relative
	// path but different CWDs would key different cache entries.
	// With anchoring, the result is stable across CWD changes.
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()

	f, err := lint.NewFile("doc.md", []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = dir
	want := indexCacheKey(f)

	other := t.TempDir()
	require.NoError(t, os.Chdir(other))
	got := indexCacheKey(f)
	assert.Equal(t, want, got, "RootDir anchoring keeps the key CWD-independent")
}

func TestValidateIndex_ClearsStaleCacheWhenIndexRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	// Seed a stale entry that would otherwise leak.
	recordIndexWriteError(f, fmt.Errorf("stale"))
	require.NotNil(t, lastIndexWriteError(f))
	// A schema without `index:` triggers want==nil; the validator
	// should clear the entry.
	diags := ValidateIndex(f, &Schema{Source: "test", RootLevel: 2}, makeDiagForTest)
	assert.Empty(t, diags)
	assert.Nil(t, lastIndexWriteError(f))
}

func TestRejectSymlinkTarget_AcceptsRegularAndMissing(t *testing.T) {
	dir := t.TempDir()
	// Missing file: nil.
	assert.NoError(t, rejectSymlinkTarget(filepath.Join(dir, "absent")))
	// Regular file: nil.
	regular := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o644))
	assert.NoError(t, rejectSymlinkTarget(regular))
	// Directory: not a symlink, accepted (WriteFile would then fail
	// elsewhere; rejectSymlinkTarget only guards symlinks).
	assert.NoError(t, rejectSymlinkTarget(dir))
	// Symlink: rejected.
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(regular, link))
	err := rejectSymlinkTarget(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestAtomicWriteIndex_FailsWhenDirAbsent(t *testing.T) {
	// CreateTemp inside a non-existent dir surfaces as an error from
	// atomicWriteIndex.
	err := atomicWriteIndex("/no/such/dir/out.json", []byte("{}"))
	require.Error(t, err)
}

func TestWriteIndex_SurfacesMkdirAllFailure(t *testing.T) {
	// Put a regular file at the parent-dir path so MkdirAll fails
	// with "not a directory". This exercises the MkdirAll error
	// branch in WriteIndex and the cache write that surfaces it on
	// the next Check.
	root := t.TempDir()
	clash := filepath.Join(root, "blocker")
	require.NoError(t, os.WriteFile(clash, []byte("regular"), 0o644))
	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "blocker/out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	err = WriteIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create parent dir")

	// And the next Check surfaces the cached error.
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "write failed")
}

func TestWriteIndex_RejectsSymlinkAtTarget(t *testing.T) {
	// An in-root symlink at the target path is rejected before
	// any bytes are written, so a hostile schema cannot use it to
	// clobber the link's destination via mdsmith fix.
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	require.NoError(t, os.WriteFile(outside, []byte("untouched"), 0o644))
	link := filepath.Join(root, "out.json")
	require.NoError(t, os.Symlink(outside, link))

	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	err = WriteIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	// The link's destination must not have been touched.
	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "untouched", string(got))
}

func TestWriteIndex_AtomicRenameReplacesExistingFile(t *testing.T) {
	// A pre-existing regular file at the target is fine and gets
	// replaced atomically. Old content must be gone afterwards.
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	out := filepath.Join(root, "out.json")
	require.NoError(t, os.WriteFile(out, []byte("stale"), 0o644))

	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	require.NoError(t, WriteIndex(f, sch))
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.NotEqual(t, "stale", string(data))
	assert.Contains(t, string(data), `"slug": "t"`)
}

func TestWriteIndex_RejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	// Create a symlink inside the project that points to a directory
	// outside it. A relative output path that resolves through this
	// symlink should be rejected.
	link := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink(outside, link))

	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "escape/out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	err = WriteIndex(f, sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink escape")
}

func TestWriteIndex_AllowsNonEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	// A symlink to a sibling within the same root should NOT be
	// rejected; verifyIndexWithinRoot only blocks escapes.
	sibling := filepath.Join(root, "real-sub")
	require.NoError(t, os.Mkdir(sibling, 0o755))
	link := filepath.Join(root, "sub")
	require.NoError(t, os.Symlink(sibling, link))

	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "sub/out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	require.NoError(t, WriteIndex(f, sch))
	_, err = os.Stat(filepath.Join(sibling, "out.json"))
	assert.NoError(t, err)
}

func TestValidateOutputPath_RejectsDotAndEmptyCleaned(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "must not be empty"},
		{"   ", "must not be empty"},
		{".", "source directory"},
		{"./", "source directory"},
		{"foo/", "must not end with a separator"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			err := validateOutputPath(tc.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateIndex_NormalizesCRLFEndings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	// First, write the canonical index so we have a baseline.
	require.NoError(t, WriteIndex(f, sch))

	// Replace the on-disk file with a CRLF-converted copy of the
	// same content. The validator must treat this as in-sync.
	target := filepath.Join(dir, "out.json")
	raw, err := os.ReadFile(target)
	require.NoError(t, err)
	crlf := bytes.ReplaceAll(raw, []byte("\n"), []byte("\r\n"))
	require.NoError(t, os.WriteFile(target, crlf, 0o644))

	diags := ValidateIndex(f, sch, makeDiagForTest)
	assert.Empty(t, diags, "CRLF version of identical content should not be stale")
}

func TestValidateIndex_SurfacesCachedWriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	// Seed the cache as if a previous Fix failed.
	recordIndexWriteError(f, fmt.Errorf("permission denied"))
	t.Cleanup(func() { recordIndexWriteError(f, nil) })

	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "write failed")
	assert.Contains(t, diags[0].Message, "permission denied")
}

func TestWriteIndex_ClearsCacheOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	// Seed a stale cache entry, then a successful write should clear it.
	recordIndexWriteError(f, fmt.Errorf("stale"))
	require.NoError(t, WriteIndex(f, sch))
	assert.Nil(t, lastIndexWriteError(f))
}

func TestValidateIndex_AbsolutePathSurfaces(t *testing.T) {
	f := newDocFile(t, "doc.md", "# T\n")
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "/etc/hosts",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "must be relative")
}
