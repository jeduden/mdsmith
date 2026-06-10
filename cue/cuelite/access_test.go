package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValue_Exists(t *testing.T) {
	t.Run("a compiled value exists", func(t *testing.T) {
		v, err := Compile(`{a: string}`)
		require.NoError(t, err)
		assert.True(t, v.Exists())
	})
	t.Run("a zero value does not exist", func(t *testing.T) {
		assert.False(t, Value{}.Exists())
	})
	t.Run("a bottom value does not exist", func(t *testing.T) {
		assert.False(t, bottom(errZeroValue).Exists())
	})
	t.Run("a looked-up present leaf exists", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a": 1}`))
		require.NoError(t, err)
		leaf, ok := v.LookupPath(MakePath("a"))
		require.True(t, ok)
		assert.True(t, leaf.Exists())
	})
	t.Run("a looked-up absent leaf does not exist", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a": 1}`))
		require.NoError(t, err)
		leaf, ok := v.LookupPath(MakePath("missing"))
		assert.False(t, ok)
		assert.False(t, leaf.Exists())
	})
}

func TestValue_LookupPath(t *testing.T) {
	t.Run("an existing leaf is found", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"meta": {"status": "done"}}`))
		require.NoError(t, err)
		leaf, ok := v.LookupPath(MakePath("meta", "status"))
		require.True(t, ok)
		s, serr := leaf.String()
		require.NoError(t, serr)
		assert.Equal(t, "done", s)
	})
	t.Run("a missing leaf reports not found", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"meta": {"status": "done"}}`))
		require.NoError(t, err)
		_, ok := v.LookupPath(MakePath("meta", "missing"))
		assert.False(t, ok)
	})
	t.Run("an empty path returns the receiver", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a": 1}`))
		require.NoError(t, err)
		got, ok := v.LookupPath(MakePath())
		require.True(t, ok)
		assert.True(t, got.Exists())
	})
	t.Run("lookup on a bottom is a bottom", func(t *testing.T) {
		_, ok := bottom(errZeroValue).LookupPath(MakePath("a"))
		assert.False(t, ok)
	})
	t.Run("a data key needing quotes is looked up verbatim", func(t *testing.T) {
		// MakePath stores "my-key" verbatim; LookupPath must find it without
		// the caller round-tripping through ParsePath.
		v, err := CompileJSON([]byte(`{"my-key": 1}`))
		require.NoError(t, err)
		_, ok := v.LookupPath(MakePath("my-key"))
		assert.True(t, ok)
	})
	t.Run("a looked-up subtree keeps rebuildable provenance for a cross-context unify", func(t *testing.T) {
		// A section-level lookup against a cached schema must still unify into
		// another context. The lookup result retains its root source plus the
		// path, so Unify can rebuild it — proving the provenance decision.
		data, err := CompileJSON([]byte(`{"meta": {"status": "✅"}}`))
		require.NoError(t, err)
		sub, ok := data.LookupPath(MakePath("meta"))
		require.True(t, ok)
		// schema lives in a different context; unifying the looked-up subtree
		// into it must keep the "✅" constraint, so a conflicting schema rejects.
		schema, err := Compile(`{status: "🔲"}`)
		require.NoError(t, err)
		assert.Error(t, schema.Unify(sub).Validate())
	})
}

func TestValue_Fields(t *testing.T) {
	t.Run("a struct yields its fields with selectors and values", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a": 1, "b": "x"}`))
		require.NoError(t, err)
		fields := v.Fields()
		require.Len(t, fields, 2)
		got := map[string]bool{}
		for _, f := range fields {
			got[f.Selector] = true
			assert.True(t, f.Value.Exists())
		}
		assert.True(t, got["a"])
		assert.True(t, got["b"])
	})
	t.Run("a scalar yields no fields", func(t *testing.T) {
		v, err := CompileJSON([]byte(`42`))
		require.NoError(t, err)
		assert.Empty(t, v.Fields())
	})
	t.Run("a bottom yields no fields", func(t *testing.T) {
		assert.Empty(t, bottom(errZeroValue).Fields())
	})
	t.Run("a nested struct field yields its own fields", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"meta": {"status": "x"}}`))
		require.NoError(t, err)
		fields := v.Fields()
		require.Len(t, fields, 1)
		assert.Equal(t, "meta", fields[0].Selector)
		inner := fields[0].Value.Fields()
		require.Len(t, inner, 1)
		assert.Equal(t, "status", inner[0].Selector)
	})
	t.Run("a key needing quotes is reported verbatim and round-trips through MakePath", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"my-key": 1}`))
		require.NoError(t, err)
		fields := v.Fields()
		require.Len(t, fields, 1)
		assert.Equal(t, "my-key", fields[0].Selector)
		_, ok := v.LookupPath(MakePath(fields[0].Selector))
		assert.True(t, ok, "a Fields() selector must look up via MakePath")
	})
}

func TestValue_Err(t *testing.T) {
	t.Run("a successfully compiled non-concrete value has no error", func(t *testing.T) {
		// {id: string} compiles fine but is not concrete; Err reports the
		// COMPILE status only, so it is nil even though Validate would fail.
		v, err := Compile(`{id: string}`)
		require.NoError(t, err)
		assert.NoError(t, v.Err())
	})
	t.Run("a compile failure surfaces through Err", func(t *testing.T) {
		v, compileErr := Compile(`{id: =}`)
		require.Error(t, compileErr)
		require.Error(t, v.Err())
	})
	t.Run("a zero value reports errZeroValue", func(t *testing.T) {
		err := Value{}.Err()
		require.Error(t, err)
		assert.ErrorIs(t, err, errZeroValue)
	})
	t.Run("a conflicting unify result is a bottom with an error", func(t *testing.T) {
		schema, err := Compile(`{a: "x"}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"a": "y"}`))
		require.NoError(t, err)
		// CUE reduces the conflict to a bottom value, so Err surfaces it —
		// matching cue.Value.Err, which reports a reduced-to-bottom value.
		assert.Error(t, schema.Unify(data).Err())
	})
}

func TestValue_String(t *testing.T) {
	t.Run("a concrete string returns its value", func(t *testing.T) {
		v, err := CompileJSON([]byte(`"hello"`))
		require.NoError(t, err)
		s, err := v.String()
		require.NoError(t, err)
		assert.Equal(t, "hello", s)
	})
	t.Run("a non-string errors", func(t *testing.T) {
		v, err := CompileJSON([]byte(`42`))
		require.NoError(t, err)
		_, err = v.String()
		assert.Error(t, err)
	})
	t.Run("a bottom errors with its reason", func(t *testing.T) {
		_, err := bottom(errZeroValue).String()
		require.Error(t, err)
		assert.ErrorIs(t, err, errZeroValue)
	})
}

func TestValue_Decode(t *testing.T) {
	t.Run("decodes a struct into a map", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a": 1, "b": "x"}`))
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, v.Decode(&out))
		assert.EqualValues(t, 1, out["a"])
		assert.Equal(t, "x", out["b"])
	})
	t.Run("a bottom errors with its reason", func(t *testing.T) {
		var out map[string]any
		err := bottom(errZeroValue).Decode(&out)
		require.Error(t, err)
		assert.ErrorIs(t, err, errZeroValue)
	})
	t.Run("a conflicting value errors", func(t *testing.T) {
		// A schema/data conflict reduces to bottom; Decode then surfaces the
		// error rather than filling out with a zero value.
		schema, err := Compile(`{a: "x"}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"a": "y"}`))
		require.NoError(t, err)
		var out map[string]any
		assert.Error(t, schema.Unify(data).Decode(&out))
	})
}
