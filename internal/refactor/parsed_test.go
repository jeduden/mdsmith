package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSource_SplitsFrontMatterWithoutParsing(t *testing.T) {
	n := countParses(t)
	src := []byte("---\ntitle: x\n---\n# H\n")
	ps := parseSource(src)
	assert.Equal(t, src, ps.source)
	assert.Equal(t, "# H\n", string(ps.body))
	assert.Equal(t, 3, ps.fmOffset)
	assert.Equal(t, 0, *n, "wrapping parses nothing")
}

func TestParsedSource_RootParsesOnce(t *testing.T) {
	n := countParses(t)
	ps := parseSource([]byte("# H\n"))
	r := ps.root()
	require.NotNil(t, r)
	assert.Same(t, r, ps.root(), "the AST is cached")
	assert.Equal(t, 1, *n)
}

func TestParsedSource_RefDefs(t *testing.T) {
	n := countParses(t)
	assert.Empty(t, parseSource([]byte("# H\n\nno defs\n")).refDefs())
	assert.Equal(t, 0, *n, "a body without `]:` is never parsed for defs")

	ps := parseSource([]byte("[a]: u\n\n```\n[b]: v\n```\n"))
	defs := ps.refDefs()
	require.Len(t, defs, 1, "the fenced look-alike is not a def")
	assert.Equal(t, "a", defs[0].normLabel)
	_ = ps.refDefs()
	_ = ps.root()
	assert.Equal(t, 1, *n, "defs and root share one parse")
}
