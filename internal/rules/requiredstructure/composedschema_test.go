package requiredstructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ComposedSchema is the exported seam the `extract` subcommand uses
// to obtain the same composed schema MDS020 validates against. With
// no configured sources it returns (nil, nil); with an inline
// source it returns that schema.
func TestComposedSchema(t *testing.T) {
	f, err := lint.NewFile("doc.md", []byte("# T\n"))
	require.NoError(t, err)

	r := &Rule{}
	sch, err := r.ComposedSchema(f)
	require.NoError(t, err)
	assert.Nil(t, sch)

	require.NoError(t, r.ApplySettings(map[string]any{
		"inline-schema": map[string]any{
			"sections": []any{map[string]any{"heading": "Goal"}},
		},
	}))
	sch, err = r.ComposedSchema(f)
	require.NoError(t, err)
	require.NotNil(t, sch)
	assert.False(t, sch.IsEmpty())
}
