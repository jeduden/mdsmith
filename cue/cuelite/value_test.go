package cuelite

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompile(t *testing.T) {
	t.Run("valid source", func(t *testing.T) {
		v, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid source", func(t *testing.T) {
		v, err := Compile(`{status: =}`)
		require.Error(t, err)
		// A compile failure yields a bottom Value whose Validate replays
		// the compile error, so a caller that ignores the error still
		// cannot mistake it for an accepting value.
		assert.Equal(t, err, v.Validate())
	})
}

func TestCompileJSON(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid json", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{not json`))
		require.Error(t, err)
		assert.Equal(t, err, v.Validate())
	})
	t.Run("unquoted key rejected as non-JSON", func(t *testing.T) {
		// CUE accepts an unquoted key; strict JSON does not. CompileJSON
		// must reject it so CUE source cannot slip in through the data arm.
		_, err := CompileJSON([]byte(`{n: 3}`))
		require.Error(t, err)
	})
	t.Run("cue expression rejected as non-JSON", func(t *testing.T) {
		_, err := CompileJSON([]byte(`{"n": >=0}`))
		require.Error(t, err)
	})
	t.Run("duplicate key builds to a bottom and reports an error", func(t *testing.T) {
		// {"a":1,"a":2} extracts as JSON but unifies to a conflicting bottom;
		// CompileJSON must surface that as a Go error, matching Compile, not
		// return (Value, nil).
		v, err := CompileJSON([]byte(`{"a":1,"a":2}`))
		require.Error(t, err)
		assert.Equal(t, err, v.Validate())
	})
}

func TestValue_Unify(t *testing.T) {
	t.Run("merges two values across their contexts", func(t *testing.T) {
		schema, err := Compile(`{status: string}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)

		assert.NoError(t, schema.Unify(data).Validate())
	})
	t.Run("bottom receiver absorbs", func(t *testing.T) {
		bad, compileErr := Compile(`{status: =}`)
		require.Error(t, compileErr)
		ok, err := Compile(`{status: string}`)
		require.NoError(t, err)
		// A bottom receiver must not panic; it propagates its compile error.
		assert.Equal(t, compileErr, bad.Unify(ok).Validate())
	})
	t.Run("bottom operand absorbs", func(t *testing.T) {
		ok, err := Compile(`{status: string}`)
		require.NoError(t, err)
		bad, compileErr := Compile(`{status: =}`)
		require.Error(t, compileErr)
		assert.Equal(t, compileErr, ok.Unify(bad).Validate())
	})
	t.Run("zero operand against concrete receiver absorbs", func(t *testing.T) {
		ok, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		// A concrete receiver that would validate on its own must still
		// reject when unified with a zero (uninitialized) operand, rather
		// than treating the zero Value as top and accepting.
		assert.NoError(t, ok.Validate(), "concrete receiver alone must pass")
		assert.Error(t, ok.Unify(Value{}).Validate())
	})
	t.Run("zero receiver against concrete operand absorbs", func(t *testing.T) {
		ok, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		// A zero receiver must absorb the operand as bottom, not panic on a
		// nil context and not accept.
		assert.Error(t, Value{}.Unify(ok).Validate())
	})
}

// TestValue_Unify_chained exercises rebuild's three branches when an
// operand is itself a derived Unify result: reuse-in-context, the
// chained merge that must keep constraints, and the unrebuildable
// cross-context operand that must absorb as bottom.
func TestValue_Unify_chained(t *testing.T) {
	t.Run("chained unify against a derived result keeps constraints", func(t *testing.T) {
		// a.Unify(b) is a derived Value in a's context; unifying c against it
		// must preserve b's constraint so a conflicting c is rejected, not
		// silently accepted by rebuilding the derived result into an empty
		// struct that drops every merged constraint.
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		c, err := CompileJSON([]byte(`{"status": "🔲"}`))
		require.NoError(t, err)

		ab := a.Unify(b)
		require.NoError(t, ab.Validate())
		// c conflicts with b's "✅"; the chained unify must reject.
		assert.Error(t, c.Unify(ab).Validate())
	})
	t.Run("derived result re-unified against its own root", func(t *testing.T) {
		// When the operand's compiled value already lives in the receiver's
		// context (a Unify result re-unified against its own root), rebuild
		// returns it directly with no recompile.
		a, err := Compile(`{status: string, weight: int}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		ab := a.Unify(b)
		// ab shares a's context, so a.Unify(ab) reuses ab.val directly.
		merged := a.Unify(ab)
		assert.Error(t, merged.Validate(), "weight still non-concrete")
	})
	t.Run("derived operand carried into a foreign context absorbs", func(t *testing.T) {
		// A derived Unify result has no retained source; unifying it as the
		// operand of an unrelated root cannot rebuild it, so it absorbs as a
		// bottom rather than vanishing into an empty struct.
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		derived := a.Unify(b)
		other, err := Compile(`{weight: int}`)
		require.NoError(t, err)
		assert.Error(t, other.Unify(derived).Validate())
	})
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
	t.Run("zero Value reports a bottom rather than panicking", func(t *testing.T) {
		// A zero receiver has no context; Validate must surface a bottom
		// error instead of dereferencing a nil context.
		assert.Error(t, Value{}.Validate())
	})
	t.Run("constraint conflict reports field path once", func(t *testing.T) {
		schema, err := Compile(`{meta: {status: "✅"}}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"meta": {"status": "🔲"}}`))
		require.NoError(t, err)

		verr := schema.Unify(data).Validate()
		require.Error(t, verr)
		var pe *PathError
		require.True(t, stderrors.As(verr, &pe), "Validate must return a *PathError, got %T", verr)
		assert.Equal(t, []string{"meta", "status"}, pe.Path())
		// The path must appear exactly once, not "meta.status: meta.status: …".
		assert.Equal(
			t,
			`meta.status: conflicting values "🔲" and "✅"`,
			pe.Error(),
		)
	})
	t.Run("multiple field failures report every path", func(t *testing.T) {
		schema, err := Compile(`{a: "x", b: "y"}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"a": "p", "b": "q"}`))
		require.NoError(t, err)

		verr := schema.Unify(data).Validate()
		require.Error(t, verr)
		// errors.Join exposes its leaves through Unwrap() []error.
		joined, ok := verr.(interface{ Unwrap() []error })
		require.True(t, ok, "multi-field failure must join its errors, got %T", verr)
		paths := make([][]string, 0, 2)
		for _, leaf := range joined.Unwrap() {
			var pe *PathError
			require.True(t, stderrors.As(leaf, &pe))
			paths = append(paths, pe.Path())
		}
		assert.Contains(t, paths, []string{"a"})
		assert.Contains(t, paths, []string{"b"})
	})
}
