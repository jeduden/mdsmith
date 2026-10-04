package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWikilinkIndex_Dirs(t *testing.T) {
	idx := NewWikilinkIndexFromPaths([]string{"x/sub/i.png", "y/a.md"})
	d := idx.Dirs()
	assert.True(t, d["x/sub"], "a file of any type under it")
	assert.True(t, d["x"], "at any depth")
	assert.False(t, d["x/su"], "a prefix of a name is not the directory")
	assert.False(t, d["z"])
	assert.False(t, d[""])
	assert.True(t, d["."], "the root holds every file")
	assert.Len(t, d, 4, "x/sub, x, y and the root")
	assert.Empty(t, NewWikilinkIndexFromPaths(nil).Dirs(), "an empty root holds none")
	var none *WikilinkIndex
	assert.Empty(t, none.Dirs())

	moved := idx.Moved(map[string]string{"x/sub/i.png": "z/i.png"})
	md := moved.Dirs()
	assert.False(t, md["x/sub"], "the overlay's key hides the base's")
	assert.True(t, md["z"], "a destination joins")
	assert.True(t, md["y"], "a key the overlay lacks reads from the base")
}

func TestAddDirs(t *testing.T) {
	dirs := map[string]bool{}
	addDirs(dirs, "a/b/c.md")
	assert.Equal(t, map[string]bool{"a/b": true, "a": true, ".": true}, dirs)
	addDirs(dirs, "a/d.md")
	assert.Len(t, dirs, 3, "an ancestor already there stops the walk")
	addDirs(dirs, "/r.md")
	assert.True(t, dirs["/"], "a rooted path stops at /")
}
