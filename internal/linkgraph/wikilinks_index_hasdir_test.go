package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWikilinkIndex_HasDir(t *testing.T) {
	idx := NewWikilinkIndexFromPaths([]string{"x/sub/i.png", "y/a.md"})
	assert.True(t, idx.HasDir("x/sub"), "a file of any type under it")
	assert.True(t, idx.HasDir("x"), "at any depth")
	assert.False(t, idx.HasDir("x/su"), "a prefix of a name is not the directory")
	assert.False(t, idx.HasDir("z"))
	assert.False(t, idx.HasDir(""))
	var none *WikilinkIndex
	assert.False(t, none.HasDir("x"))

	moved := idx.Moved(map[string]string{"x/sub/i.png": "z/i.png"})
	assert.False(t, moved.HasDir("x/sub"), "the overlay's key hides the base's")
	assert.True(t, moved.HasDir("z"), "a destination joins")
	assert.True(t, moved.HasDir("y"), "a key the overlay lacks reads from the base")
}
