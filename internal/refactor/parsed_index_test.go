package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsedSource_Index(t *testing.T) {
	ps := parseSource([]byte("a\nbb\nccc\n"))
	idx := ps.index()
	assert.Equal(t, 1, idx.lineOfOffset(0))
	assert.Equal(t, 3, idx.lineOfOffset(5))
	// The index is built once and shared by every later caller.
	again := ps.index()
	assert.Same(t, &idx.starts[0], &again.starts[0])
}
