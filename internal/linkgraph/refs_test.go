package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestExtractLinkRefs_Basic(t *testing.T) {
	src := "# Doc\n\nSee [first][a] and [second][B].\n\n[a]: a.md\n[b]: b.md\n"
	f := newFile(t, src)
	refs := ExtractLinkRefs(f)
	require.Len(t, refs, 2)
	assert.Equal(t, "first", refs[0].Text)
	assert.Equal(t, "a", refs[0].Label)
	assert.Equal(t, 3, refs[0].Line)
	assert.Equal(t, "second", refs[1].Text)
	// util.ToLinkReference lowercases and collapses whitespace.
	assert.Equal(t, "b", refs[1].Label)
}

func TestExtractLinkRefs_SkipsDirectLinks(t *testing.T) {
	src := "# Doc\n\n[direct](a.md) and [ref][r].\n\n[r]: x.md\n"
	f := newFile(t, src)
	refs := ExtractLinkRefs(f)
	require.Len(t, refs, 1)
	assert.Equal(t, "ref", refs[0].Text)
}

func TestExtractLinkRefs_NilFile(t *testing.T) {
	assert.Nil(t, ExtractLinkRefs(nil))
}

func TestExtractLinkRefs_LinesAreBodyRelative(t *testing.T) {
	src := []byte("---\ntitle: x\n---\nSee [r][lbl].\n\n[lbl]: x.md\n")
	f, err := lint.NewFileFromSource("file.md", src, true)
	require.NoError(t, err)
	refs := ExtractLinkRefs(f)
	require.Len(t, refs, 1)
	// The ref is on body line 1 (after a 3-line FM strip). Callers add
	// f.LineOffset themselves; ExtractLinkRefs must not pre-apply it.
	assert.Equal(t, 1, refs[0].Line)
	assert.Equal(t, 3, f.LineOffset, "front matter occupies 3 lines")
}
