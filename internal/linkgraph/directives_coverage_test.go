package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
)

// TestExtractDirectives_BuildEmptyInputEntrySkipped covers the build
// branch where an `inputs:` entry is empty after trim — that entry
// contributes no edge (dedicated lint rules report the user-facing
// diagnostic). A trailing empty entry alongside a valid one still
// yields exactly one edge.
func TestExtractDirectives_BuildEmptyInputEntrySkipped(t *testing.T) {
	src := "# T\n\n<?build\nrecipe: r\ninputs:\n  - good.md\n  - \"\"\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	assert.Equal(t, "good.md", edges[0].Path)
}

// TestExtractDirectives_CatalogEmptyGlobsValue covers the
// splitCatalogGlobs empty-string branch. A catalog directive whose
// glob is an empty string still produces a DirectiveCatalog edge,
// but its Globs slice is nil.
func TestExtractDirectives_CatalogEmptyGlobsValue(t *testing.T) {
	src := "# T\n\n<?catalog\nglob: \"\"\n?>\n<?/catalog?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	assert.Equal(t, DirectiveCatalog, edges[0].Kind)
	assert.Nil(t, edges[0].Globs)
}

// TestExtractDirectives_CatalogWhitespaceOnlyGlobsValue covers the
// splitCatalogGlobs all-empty-after-trim branch. A glob list of
// whitespace entries normalises to nil rather than a slice of empty
// strings.
func TestExtractDirectives_CatalogWhitespaceOnlyGlobsValue(t *testing.T) {
	// gensection joins list entries with `\n` — a list of two
	// whitespace strings becomes "   \n   " before splitCatalogGlobs.
	src := "# T\n\n<?catalog\nglob:\n  - \"   \"\n  - \"\"\n?>\n<?/catalog?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	assert.Equal(t, DirectiveCatalog, edges[0].Kind)
	assert.Nil(t, edges[0].Globs,
		"all-empty entries after TrimSpace yield nil, not a slice of empties")
}

// TestExtractDirectives_SingleLineMalformedReturnsEmpty covers the
// extractPIBody single-line branch (Lines().Len() <= 1) — single-line
// PIs have no YAML body, so include/build/catalog with no parameters
// at all produce no edge.
func TestExtractDirectives_SingleLineNoBody(t *testing.T) {
	// `<?include?>` is single-line and has no body; the missing
	// required `file:` parameter means no edge.
	src := "# T\n\n<?include?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f))
}

// TestLineOfOffset_NegativeAndOversize covers the two defensive
// branches in lineOfOffset (negative offset and offset >= len).
// The helper is package-private; we exercise it directly because
// the public entry points always pass valid offsets.
func TestLineOfOffset_NegativeAndOversize(t *testing.T) {
	src := []byte("abc\ndef\n")
	assert.Equal(t, 1, lineOfOffset(src, -1),
		"negative offset returns line 1")
	assert.Equal(t, 3, lineOfOffset(src, 1000),
		"oversize offset clamps to len(source)")
	assert.Equal(t, 2, lineOfOffset(src, 4),
		"normal in-range offset returns the right line")
}

// TestDirectivePILine_NoLines covers the defensive branch where a
// processing-instruction has no Lines(). Goldmark guarantees Lines()
// is non-empty for any parsed PI, so we construct the case directly
// using a zero-value piparser.ProcessingInstruction; the helper falls
// back to line 1.
func TestDirectivePILine_NoLines(t *testing.T) {
	pi := &piparser.ProcessingInstruction{}
	// Synthesize a *lint.File so directivePILine has something to
	// dereference. f.LineOfOffset is unreachable from the empty-Lines
	// branch.
	source := []byte("# T\n")
	f, err := lint.NewFile("test.md", source)
	require.NoError(t, err)
	assert.Equal(t, 1, directivePILine(f, pi))
}

// TestExtractDirectives_NonStringParamRejected covers the
// ValidateStringParams diagnostic branch — a YAML body that parses
// but has a non-string-typed required parameter (e.g. a mapping
// where a string is expected) produces no edge.
func TestExtractDirectives_NonStringParamRejected(t *testing.T) {
	// `file:` becomes a YAML mapping rather than a string.
	// gensection.ParseYAMLBody parses cleanly, but
	// gensection.ValidateStringParams rejects the non-string value
	// and ExtractDirectives must drop the directive.
	src := "# T\n\n<?include\nfile:\n  nested: x\n?>\n<?/include?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f),
		"non-string file: param must not produce an edge")
}
