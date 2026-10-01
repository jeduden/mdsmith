package yamlutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestDecodeNoPanic covers both outcomes: a normal decode passes
// through, and a yaml.v3 runtime panic comes back as an error.
func TestDecodeNoPanic(t *testing.T) {
	t.Parallel()
	parse := func(t *testing.T, src string) *yaml.Node {
		t.Helper()
		var n yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(src), &n))
		return &n
	}

	t.Run("decodes", func(t *testing.T) {
		t.Parallel()
		var m map[string]string
		require.NoError(t, decodeNoPanic(parse(t, "a: b\n"), &m))
		assert.Equal(t, map[string]string{"a": "b"}, m)
	})

	t.Run("panic becomes error", func(t *testing.T) {
		t.Parallel()
		var m map[string]any
		err := decodeNoPanic(parse(t, "? [a, b]\n: c\n<<: {x: y}\n"), &m)
		assert.ErrorContains(t, err, "unhashable")
	})
}

// TestDecodeNodeSafe: a parsed node decodes, a decode panic is an
// error, and a nil node is an error rather than a crash.
func TestDecodeNodeSafe(t *testing.T) {
	t.Parallel()
	t.Run("decodes", func(t *testing.T) {
		t.Parallel()
		doc, err := UnmarshalNodeSafe([]byte("a: b\n"))
		require.NoError(t, err)
		var m map[string]string
		require.NoError(t, DecodeNodeSafe(&doc, &m))
		assert.Equal(t, map[string]string{"a": "b"}, m)
	})
	t.Run("panic becomes error", func(t *testing.T) {
		t.Parallel()
		doc, err := UnmarshalNodeSafe([]byte("? [a, b]\n: c\n<<: {x: y}\n"))
		require.NoError(t, err)
		var m map[string]any
		assert.ErrorContains(t, DecodeNodeSafe(&doc, &m), "unhashable")
	})
	t.Run("nil node", func(t *testing.T) {
		t.Parallel()
		var m map[string]any
		assert.Error(t, DecodeNodeSafe(nil, &m))
	})
}
