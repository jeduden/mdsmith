package requiredstructure

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
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

// A file with no front matter has nothing to decode, so it must not
// pay for a memo entry either: every file of every kind reaches this
// on the MDS020 check path, under the rule's allocation budget.
func TestCachedDocFrontMatterRaw_NoFrontMatterSkipsMemo(t *testing.T) {
	f := newTestFile(t, "doc.md", "# X\n")
	raw, diags := cachedDocFrontMatterRaw(f)
	assert.Nil(t, raw)
	assert.Nil(t, diags)
	built := false
	f.MemoFile(docFrontMatterMemoKey, func(*lint.File) any {
		built = true
		return nil
	})
	assert.True(t, built, "no memo entry may exist for a file without front matter")
}

// MemoFile keeps a nil value when its builder panicked (the panic
// still marks the entry done). The memo read must not turn that into
// a second panic on the next call; it decodes the block directly.
func TestCachedDocFrontMatterRaw_ToleratesUnexpectedMemoValue(t *testing.T) {
	f := newTestFile(t, "doc.md", "---\nname: x\n---\n# X\n")
	f.MemoFile(docFrontMatterMemoKey, func(*lint.File) any { return nil })
	var raw map[string]any
	var diags []lint.Diagnostic
	require.NotPanics(t, func() { raw, diags = cachedDocFrontMatterRaw(f) })
	assert.Equal(t, "x", raw["name"])
	assert.Empty(t, diags)
}
