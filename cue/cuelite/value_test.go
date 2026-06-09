package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompile(t *testing.T) {
	t.Run("valid source", func(t *testing.T) {
		v, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		require.NotNil(t, v)
	})
	t.Run("invalid source", func(t *testing.T) {
		v, err := Compile(`{status: =}`)
		require.Error(t, err)
		assert.Nil(t, v)
	})
}

func TestCompileJSON(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		require.NotNil(t, v)
	})
	t.Run("invalid json", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{not json`))
		require.Error(t, err)
		assert.Nil(t, v)
	})
}

func TestValue_Unify(t *testing.T) {
	schema, err := Compile(`{status: string}`)
	require.NoError(t, err)
	data, err := CompileJSON([]byte(`{"status": "✅"}`))
	require.NoError(t, err)

	merged := schema.Unify(data)
	require.NotNil(t, merged)
	assert.NoError(t, merged.Validate())
}

func TestValue_Validate(t *testing.T) {
	t.Run("concrete value passes", func(t *testing.T) {
		schema, err := Compile(`{status: string}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "done"}`))
		require.NoError(t, err)
		assert.NoError(t, schema.Unify(data).Validate())
	})
	t.Run("non-concrete value fails", func(t *testing.T) {
		schema, err := Compile(`{status: string}`)
		require.NoError(t, err)
		assert.Error(t, schema.Validate())
	})
	t.Run("constraint conflict reports field path", func(t *testing.T) {
		schema, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "🔲"}`))
		require.NoError(t, err)

		verr := schema.Unify(data).Validate()
		require.Error(t, verr)
		pe, ok := verr.(*PathError)
		require.True(t, ok, "Validate must return a *PathError, got %T", verr)
		assert.Equal(t, []string{"status"}, pe.Path())
	})
}
