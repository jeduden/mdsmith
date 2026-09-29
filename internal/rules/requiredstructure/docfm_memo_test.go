package requiredstructure

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The kind path-pattern check and the schema check both need the
// document's front matter. cachedDocFrontMatterRaw decodes it once per
// file and hands every caller the same map.
func TestCachedDocFrontMatterRaw_DecodesOncePerFile(t *testing.T) {
	f := newTestFile(t, "doc.md", "---\nname: x\n---\n# X\n")
	a, aDiags := cachedDocFrontMatterRaw(f)
	b, bDiags := cachedDocFrontMatterRaw(f)
	require.Equal(t, "x", a["name"])
	assert.Empty(t, aDiags)
	assert.Empty(t, bDiags)
	assert.Equal(t, reflect.ValueOf(a).Pointer(), reflect.ValueOf(b).Pointer(),
		"the second call must reuse the first decode")
}

func TestCachedDocFrontMatterRaw_KeepsParseDiagnostic(t *testing.T) {
	f := newTestFile(t, "doc.md", "---\nname: [x\n---\n# X\n")
	_, d1 := cachedDocFrontMatterRaw(f)
	_, d2 := cachedDocFrontMatterRaw(f)
	require.Len(t, d1, 1)
	assert.Contains(t, d1[0].Message, "invalid YAML")
	assert.Equal(t, d1, d2)
}
