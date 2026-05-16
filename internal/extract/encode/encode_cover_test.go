package encode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncode_UnknownFormat(t *testing.T) {
	_, err := Encode(Format("lua"), map[string]any{})
	require.Error(t, err)
}

// A func value is unserialisable; json and msgpack return an error
// for it (rather than panicking), exercising those error returns.
func TestEncode_SerializationErrors(t *testing.T) {
	bad := map[string]any{"f": func() {}}
	for _, f := range []Format{JSON, Msgpack} {
		_, err := Encode(f, bad)
		assert.Error(t, err, "format %s should error on a func", f)
	}
}
