package schema

import (
	"errors"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaKeyForPath_Edges exercises the three return
// branches: empty path, required-key match, optional-key
// match, and no-match fallback.
func TestSchemaKeyForPath_Edges(t *testing.T) {
	sch := &Schema{
		Frontmatter: map[string]string{
			"id":           `int`,
			"description?": `string`,
		},
	}
	assert.Equal(t, "", schemaKeyForPath(sch, nil))
	assert.Equal(t, "id", schemaKeyForPath(sch, []string{"id"}))
	assert.Equal(t, "description?", schemaKeyForPath(sch, []string{"description"}))
	assert.Equal(t, "", schemaKeyForPath(sch, []string{"unknown"}))
}

// TestLookupConstraint_FallsBackToEmpty regresses the
// no-match branch: a CUE error path whose first segment
// isn't in the schema's Frontmatter map returns the empty
// string so the caller falls into the "extra field" path.
func TestLookupConstraint_FallsBackToEmpty(t *testing.T) {
	sch := &Schema{
		Frontmatter: map[string]string{"id": `int`},
	}
	assert.Equal(t, "", lookupConstraint(sch, []string{"unknown"}))
}

// TestLookupFM_EmptyPathReportsAbsent regresses the
// len(path)==0 early-return.
func TestLookupFM_EmptyPathReportsAbsent(t *testing.T) {
	v, ok := lookupFM(map[string]any{"a": 1}, nil)
	assert.False(t, ok)
	assert.Nil(t, v)
}

// TestLookupFM_NonMapStopsTraversal regresses the
// default branch in the type switch: a string value where
// the path expects a deeper key returns absent.
func TestLookupFM_NonMapStopsTraversal(t *testing.T) {
	fm := map[string]any{"a": "scalar"}
	_, ok := lookupFM(fm, []string{"a", "deeper"})
	assert.False(t, ok)
}

// TestSchemaRef_EmptySourceFallback covers the empty-source
// branch: when sch.Source is unset, the helper renders the
// generic "schema" label so every diagnostic carries a
// reference suffix.
func TestSchemaRef_EmptySourceFallback(t *testing.T) {
	sch := &Schema{}
	assert.Equal(t, "schema", FormatSchemaRef(sch, ""))
}

// TestSchemaRef_LineMissingFromMap covers the branch where
// the key is in FrontmatterLines as zero; without a line
// the helper falls back to just the source label.
func TestSchemaRef_LineMissingFromMap(t *testing.T) {
	sch := &Schema{
		Source: "plan/proto.md",
		FrontmatterLines: map[string]int{
			"status": 0, // explicitly zero — treated as unknown
		},
	}
	assert.Equal(t, "plan/proto.md", FormatSchemaRef(sch, "status"))
}

// TestValidateFrontmatterDiags_NilDocFMTreatedAsEmpty
// covers the docFM==nil branch: passing nil should be
// equivalent to passing an empty map, exposing missing
// required-field diagnostics rather than crashing.
func TestValidateFrontmatterDiags_NilDocFMTreatedAsEmpty(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{"id": `int`},
	}
	sch, err := ParseInline(raw, "kind t")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	diags := ValidateFrontmatterDiags(doc, sch, nil, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "id: got <missing>")
}

// TestFmDiagLine_EmptyPath covers the len(path)==0 early
// return.
func TestFmDiagLine_EmptyPath(t *testing.T) {
	f, err := lint.NewFileFromSource("doc.md", []byte("# T\n"), true)
	require.NoError(t, err)
	assert.Equal(t, 1, fmDiagLine(f, nil, map[string]int{"x": 2}))
}

// TestFmDiagLine_KeyMissingFromMap covers the !ok branch:
// the path's first segment isn't in keyLines, so the helper
// falls back to line 1.
func TestFmDiagLine_KeyMissingFromMap(t *testing.T) {
	f, err := lint.NewFileFromSource("doc.md", []byte("# T\n"), true)
	require.NoError(t, err)
	assert.Equal(t, 1, fmDiagLine(f, []string{"absent"}, map[string]int{"x": 2}))
}

// TestDocFrontmatterKeyLines_NoFrontMatterReturnsNil covers
// the no-FM branch: a file whose source doesn't start with
// `---\n` returns nil.
func TestDocFrontmatterKeyLines_NoFrontMatterReturnsNil(t *testing.T) {
	f, err := lint.NewFileFromSource("doc.md", []byte("# T\n"), true)
	require.NoError(t, err)
	assert.Nil(t, docFrontmatterKeyLines(f))
}

// TestParseFMBlockKeyLines_EmptyBodyReturnsNil exercises
// the empty-body branch: just the opening and closing
// fences with no YAML between them.
func TestParseFMBlockKeyLines_EmptyBodyReturnsNil(t *testing.T) {
	assert.Nil(t, parseFMBlockKeyLines([]byte("---\n---\n")))
}

// TestParseFMBlockKeyLines_InvalidYAML covers the
// UnmarshalNodeSafe error path: malformed YAML returns nil
// so callers degrade to a "no per-key line known" fallback.
func TestParseFMBlockKeyLines_InvalidYAML(t *testing.T) {
	// YAML anchors are rejected by yamlutil.UnmarshalNodeSafe;
	// the error path returns nil.
	bad := []byte("---\nfoo: &a 1\nbar: *a\n---\n")
	assert.Nil(t, parseFMBlockKeyLines(bad))
}

// TestValidateFrontmatterDiags_EmptyConstraintsReturnsNil
// covers the FrontmatterCUE()=="" early return: an empty
// frontmatter map produces no constraints, so the
// validator produces no diagnostics regardless of input.
func TestValidateFrontmatterDiags_EmptyConstraintsReturnsNil(t *testing.T) {
	sch := &Schema{}
	f, err := lint.NewFileFromSource("doc.md", []byte("# T\n"), true)
	require.NoError(t, err)
	diags := ValidateFrontmatterDiags(f, sch, map[string]any{"id": 1}, makeDiagForTest)
	assert.Nil(t, diags)
}

// TestValidateFrontmatterDiags_InvalidCUESchemaCarriesRef
// covers the schemaVal.Err() != nil branch: a malformed
// CUE expression in the schema's Frontmatter map produces
// the compileFailureDiag fallback with the schema source.
func TestValidateFrontmatterDiags_InvalidCUESchemaCarriesRef(t *testing.T) {
	sch := &Schema{
		Source: "kind bad",
		Frontmatter: map[string]string{
			"id": "int &", // syntactically invalid
		},
	}
	f, err := lint.NewFileFromSource("doc.md", []byte("# T\n"), true)
	require.NoError(t, err)
	diags := ValidateFrontmatterDiags(f, sch, map[string]any{"id": 1}, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "schema:")
	assert.Contains(t, diags[0].Message, "kind bad")
}

// TestCompileFailureDiag_FieldsRoundTrip exercises the
// helper directly so the Expected vocabulary plumbing is
// covered even when CUE never reaches that branch.
func TestCompileFailureDiag_FieldsRoundTrip(t *testing.T) {
	sch := &Schema{Source: "kind t"}
	d := compileFailureDiag(sch, "front matter", "valid front matter", errors.New("boom"))
	assert.Equal(t, "front matter", d.Field)
	assert.Equal(t, "valid front matter", d.Expected)
	assert.Contains(t, d.Actual, "boom")
	assert.Equal(t, "kind t", d.SchemaRef)
}

// TestNonBodyDiagLine_StrippedAndUnstripped exercises the
// helper directly: a file built with FM stripping returns
// a non-positive body coord (so filterGeneratedDiags can't
// match it), and a file built without stripping returns 1
// unchanged.
func TestNonBodyDiagLine_StrippedAndUnstripped(t *testing.T) {
	stripped, err := lint.NewFileFromSource("doc.md",
		[]byte("---\nfoo: 1\n---\n# Body\n"), true)
	require.NoError(t, err)
	require.Greater(t, stripped.LineOffset, 0)
	assert.LessOrEqual(t, NonBodyDiagLine(stripped), 0)

	unstripped, err := lint.NewFile("doc.md", []byte("# Body\n"))
	require.NoError(t, err)
	assert.Equal(t, 0, unstripped.LineOffset)
	assert.Equal(t, 1, NonBodyDiagLine(unstripped))
}

// TestValidateFrontmatterDiags_JSONMarshalFailureCarriesRef
// regresses the json.Marshal early-return path. A channel
// value in docFM is non-marshalable, so the validator falls
// through to the JSON-marshalable-front-matter compile
// failure diagnostic.
func TestValidateFrontmatterDiags_JSONMarshalFailureCarriesRef(t *testing.T) {
	sch := &Schema{
		Source: "kind broken",
		Frontmatter: map[string]string{
			"data": `string`,
		},
	}
	f, err := lint.NewFileFromSource("doc.md", []byte("# T\n"), true)
	require.NoError(t, err)
	docFM := map[string]any{"data": make(chan int)}
	diags := ValidateFrontmatterDiags(f, sch, docFM, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "JSON-marshalable")
	assert.Contains(t, diags[0].Message, "kind broken")
}

// TestValidateFrontmatterDiags_ExtraFieldCarriesRef
// exercises the extra-field branch of schemaDiagFromCUEError:
// an unknown key (rejected by close()) lacks a per-field
// constraint, so the diagnostic renders the value the user
// wrote plus the "not declared in schema" sentinel.
func TestValidateFrontmatterDiags_ExtraFieldCarriesRef(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id": `int`,
		},
	}
	sch, err := ParseInline(raw, "kind t")
	require.NoError(t, err)
	f, err2 := lint.NewFileFromSource("doc.md", []byte("# T\n"), true)
	require.NoError(t, err2)
	docFM := map[string]any{"id": 1, "extra": true}
	diags := ValidateFrontmatterDiags(f, sch, docFM, makeDiagForTest)
	require.NotEmpty(t, diags)
	var hasExtra bool
	for _, d := range diags {
		if strings.Contains(d.Message, "extra:") &&
			strings.Contains(d.Message, "not declared in schema") {
			hasExtra = true
		}
	}
	assert.True(t, hasExtra,
		"extra field should surface with the not-declared sentinel")
}

// TestDocFrontmatterKeyLines_SourceFallback exercises the
// branch where f.FrontMatter is empty but the file's source
// starts with `---\n`. The integration runner takes this path
// (lint.NewFile keeps FM in the source); the helper extracts
// the FM block from the source and returns the per-key
// lines.
func TestDocFrontmatterKeyLines_SourceFallback(t *testing.T) {
	src := []byte("---\nid: 1\nstatus: open\n---\n# Body\n")
	f, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)
	// NewFile (not NewFileFromSource) leaves FrontMatter
	// empty and keeps the FM in the source.
	lines := docFrontmatterKeyLines(f)
	require.NotNil(t, lines)
	assert.Equal(t, 2, lines["id"])
	assert.Equal(t, 3, lines["status"])
}

// TestDocFrontmatterKeyLines_StrippedFrontMatter covers the
// production path: lint.NewFileFromSource(..., true) leaves
// f.FrontMatter populated with the stripped block. The
// helper goes through parseFMBlockKeyLines directly without
// re-extracting from the source.
func TestDocFrontmatterKeyLines_StrippedFrontMatter(t *testing.T) {
	src := []byte("---\nid: 1\nstatus: open\n---\n# Body\n")
	f, err := lint.NewFileFromSource("doc.md", src, true)
	require.NoError(t, err)
	require.NotEmpty(t, f.FrontMatter)
	lines := docFrontmatterKeyLines(f)
	require.NotNil(t, lines)
	assert.Equal(t, 2, lines["id"])
	assert.Equal(t, 3, lines["status"])
}
