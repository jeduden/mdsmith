package encode

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncode_UnknownFormat(t *testing.T) {
	_, err := Encode(Format("lua"), map[string]any{})
	require.Error(t, err)
}

// A func value is unserialisable; json and msgpack return an error
// for it. yaml.v3 panics on a func, so its error return is driven
// with a yaml.Marshaler that fails instead.
func TestEncode_SerializationErrors(t *testing.T) {
	bad := map[string]any{"f": func() {}}
	for _, f := range []Format{JSON, Msgpack} {
		_, err := Encode(f, bad)
		assert.Error(t, err, "format %s should error on a func", f)
	}
	_, err := Encode(YAML, map[string]any{"x": yamlBomb{}})
	assert.Error(t, err)
}

type yamlBomb struct{}

func (yamlBomb) MarshalYAML() (any, error) {
	return nil, errBomb
}

var errBomb = fmt.Errorf("yaml marshal failed")
