package encode

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"gopkg.in/yaml.v3"
)

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"json", "yaml", "msgpack"} {
		f, err := ParseFormat(s)
		require.NoError(t, err)
		assert.Equal(t, Format(s), f)
	}
	_, err := ParseFormat("lua")
	require.Error(t, err)
}

func TestEncode_EquivalentAcrossFormats(t *testing.T) {
	src := map[string]any{
		"frontmatter": map[string]any{"id": "x"},
		"goal":        map[string]any{"text": "do it"},
		"step":        []any{map[string]any{"n": "1"}},
	}

	jb, err := Encode(JSON, src)
	require.NoError(t, err)
	var jv map[string]any
	require.NoError(t, json.Unmarshal(jb, &jv))

	yb, err := Encode(YAML, src)
	require.NoError(t, err)
	var yv map[string]any
	require.NoError(t, yaml.Unmarshal(yb, &yv))

	mb, err := Encode(Msgpack, src)
	require.NoError(t, err)
	var mv map[string]any
	require.NoError(t, msgpack.Unmarshal(mb, &mv))

	assert.Equal(t, jv, yv)
	assert.Equal(t, jv, mv)
}
