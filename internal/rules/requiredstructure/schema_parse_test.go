package requiredstructure

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
)

// A schema front-matter list can hold a mapping whose keys are not
// all strings (`tags: [{1: a}]`). YAML decodes that mapping to
// map[any]any, which encoding/json cannot marshal, so the list has no
// CUE literal. cueExprForValue must name the array in its error.
func TestCueExprForValue_ArrayJSONMarshalError(t *testing.T) {
	_, err := cueExprForValue([]any{map[any]any{1: "a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal array value")
}

// YAML `.nan` decodes to a float64 NaN, which JSON (and so the CUE
// literal cueExprForValue emits) cannot represent. The error must name
// the scalar rather than emit an invalid constraint.
func TestCueExprForValue_ScalarJSONMarshalError(t *testing.T) {
	_, err := cueExprForValue(math.NaN())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal scalar value")
}

// A single-line include with no parameters (`<?include ?>`) has an
// empty body, so extractPIFileParam reports no file and no error; the
// caller (resolveSchemaIncludePath) turns the empty path into the
// "missing required 'file' attribute" diagnostic.
func TestExtractPIFileParam_EmptyBody(t *testing.T) {
	src := "<?include ?>\ncontent\n<?/include?>"
	f, err := lint.NewFileFromSource("schema.md", []byte(src), true)
	require.NoError(t, err)
	var pi *piparser.ProcessingInstruction
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*piparser.ProcessingInstruction); ok {
			pi = p
			break
		}
	}
	require.NotNil(t, pi, "expected ProcessingInstruction in parsed AST")
	require.Equal(t, 1, pi.Lines().Len(), "fixture must be the single-line form")

	file, err := extractPIFileParam(pi, []byte(src))
	require.NoError(t, err)
	assert.Empty(t, file)

	_, err = resolveSchemaIncludePath(pi, []byte(src), "schema.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'file' attribute")
}
