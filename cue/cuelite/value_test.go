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
		// A compile failure yields a bottom Value whose Validate replays the
		// compile error as a path-free *PathError — preserving the message so
		// a caller that ignores the error still cannot mistake it for an
		// accepting value, while keeping the Errors invariant (a non-nil
		// Validate always decomposes to at least one *PathError).
		assertBottomError(t, v.Validate(), err.Error())
	})
}

// assertBottomError asserts that verr is a non-nil error decomposing to a
// single path-free *PathError carrying wantMsg — the shape Validate returns
// for every bottom Value, so the Errors invariant holds.
func assertBottomError(t *testing.T, verr error, wantMsg string) {
	t.Helper()
	require.Error(t, verr)
	leaves := Errors(verr)
	require.Len(t, leaves, 1)
	assert.Empty(t, leaves[0].Path())
	assert.Equal(t, wantMsg, leaves[0].Error())
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
		assertBottomError(t, v.Validate(), err.Error())
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
		assertBottomError(t, v.Validate(), err.Error())
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
		// A bottom receiver must not panic; it propagates its compile error
		// as a path-free *PathError preserving the message.
		assertBottomError(t, bad.Unify(ok).Validate(), compileErr.Error())
	})
	t.Run("bottom operand absorbs", func(t *testing.T) {
		ok, err := Compile(`{status: string}`)
		require.NoError(t, err)
		bad, compileErr := Compile(`{status: =}`)
		require.Error(t, compileErr)
		assertBottomError(t, ok.Unify(bad).Validate(), compileErr.Error())
	})
	t.Run("zero operand against concrete receiver absorbs", func(t *testing.T) {
		ok, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		// A concrete receiver that would validate on its own must still
		// reject when unified with a zero (uninitialized) operand, rather
		// than treating the zero Value as top and accepting. Pinning the
		// exact errZeroValue reason makes the operand isBottom guard load-
		// bearing: removing it leaves a (different) rebuild bottom that this
		// assertion rejects, turning the test red.
		assert.NoError(t, ok.Validate(), "concrete receiver alone must pass")
		assertBottomError(t, ok.Unify(Value{}).Validate(), errZeroValue.Error())
	})
	t.Run("zero receiver against concrete operand absorbs", func(t *testing.T) {
		ok, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		// A zero receiver must absorb the operand as bottom, not panic on a
		// nil context and not accept. The reason must be errZeroValue so the
		// receiver isBottom guard cannot be removed without going red.
		assertBottomError(t, Value{}.Unify(ok).Validate(), errZeroValue.Error())
	})
}

// TestValue_Unify_chained exercises rebuild when an operand is itself a
// derived Unify result that still lives in (or shares) the receiver's
// context: the chained merge that must keep constraints, and the reuse
// of a result re-unified against its own root. Cross-context cases are
// in TestValue_Unify_crossContext.
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
}

// TestValue_Unify_crossContext covers the order-insensitive cross-context
// unification of finding 3: a derived operand against a source-carrying
// receiver rebuilds the RECEIVER into the operand's context, so c.Unify(
// a.Unify(b)) of compatible roots validates and of conflicting roots
// rejects with the right path; only when BOTH sides are derived in
// different contexts does it absorb as a pathless bottom.
func TestValue_Unify_crossContext(t *testing.T) {
	t.Run("compatible roots validate regardless of chaining order", func(t *testing.T) {
		// schema (source) .Unify( a.Unify(data) derived ): the operand is
		// derived, the receiver still carries source, so the receiver is
		// rebuilt into the operand's context. Left-chaining is not the only
		// order that works.
		schema, err := Compile(`{status: string}`)
		require.NoError(t, err)
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		assert.NoError(t, schema.Unify(a.Unify(data)).Validate())
	})
	t.Run("conflicting roots reject with the field path", func(t *testing.T) {
		// c (source) demands "🔲"; the derived operand a.Unify(b) pins status
		// to "✅". The rebuild-the-receiver path must still surface the
		// conflict at status, not silently accept.
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		c, err := Compile(`{status: "🔲"}`)
		require.NoError(t, err)
		verr := c.Unify(a.Unify(b)).Validate()
		require.Error(t, verr)
		leaves := Errors(verr)
		require.Len(t, leaves, 1)
		assert.Equal(t, []string{"status"}, leaves[0].Path())
	})
	t.Run("source-carrying receiver keeps both constraints", func(t *testing.T) {
		// other (source) rebuilds into the derived operand's context, giving
		// {weight: int, status: "✅"}; weight is non-concrete, so Validate
		// fails — proving the constraints merged rather than absorbing.
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		other, err := Compile(`{weight: int}`)
		require.NoError(t, err)
		verr := other.Unify(a.Unify(b)).Validate()
		require.Error(t, verr)
		leaves := Errors(verr)
		require.Len(t, leaves, 1)
		assert.Equal(t, []string{"weight"}, leaves[0].Path())
	})
	t.Run("both operands derived in different contexts absorb as a bottom", func(t *testing.T) {
		// When NEITHER side retains source — both are derived Unify results in
		// different contexts — unification cannot rebuild either into the
		// other and absorbs as a pathless bottom, which Errors still surfaces.
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		c, err := Compile(`{weight: int}`)
		require.NoError(t, err)
		d, err := CompileJSON([]byte(`{"weight": 1}`))
		require.NoError(t, err)
		verr := a.Unify(b).Unify(c.Unify(d)).Validate()
		require.Error(t, verr)
		leaves := Errors(verr)
		require.Len(t, leaves, 1)
		assert.Empty(t, leaves[0].Path(), "a cross-context bottom carries no leaf path")
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
		// error instead of dereferencing a nil context. The reason must be
		// errZeroValue so the isBottom guard stays load-bearing.
		assertBottomError(t, Value{}.Validate(), errZeroValue.Error())
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
		// The concrete error shape is unspecified; read leaves through the
		// Errors accessor rather than hand-rolling a join traversal.
		leaves := Errors(verr)
		require.Len(t, leaves, 2)
		paths := make([][]string, 0, 2)
		for _, leaf := range leaves {
			paths = append(paths, leaf.Path())
		}
		assert.Contains(t, paths, []string{"a"})
		assert.Contains(t, paths, []string{"b"})
	})
}

// TestJoinValidationErrors_emptyDecomposition drives the fail-open guard
// in joinValidationErrors red/green through the package-internal seam: an
// empty leaf slice must NOT collapse to a nil error (stderrors.Join of
// nothing is nil, which a caller reads as acceptance) but instead yield a
// path-free *PathError wrapping verr's message — preserving the Validate
// invariant even if a future CUE version stops decomposing a bottom.
func TestJoinValidationErrors_emptyDecomposition(t *testing.T) {
	verr := stderrors.New("data does not satisfy schema")
	got := joinValidationErrors(nil, verr)
	require.Error(t, got, "an empty decomposition must not flatten to nil")
	leaves := Errors(got)
	require.Len(t, leaves, 1)
	assert.Empty(t, leaves[0].Path())
	assert.Equal(t, "data does not satisfy schema", leaves[0].Error())
}

// TestValidate_invariant pins the contract every consumer loop relies on:
// a non-nil Validate error always decomposes to at least one *PathError,
// so a loop over Errors never emits zero diagnostics for a failing value.
// The three bottom flavors — zero Value, a replayed compile error, and a
// cross-context derived bottom — were the gap: they returned a bare Go
// error that Errors flattened to nil. Each must now surface a *PathError
// (with an empty path, the message preserved).
func TestValidate_invariant(t *testing.T) {
	bottoms := map[string]func(t *testing.T) error{
		"zero Value": func(t *testing.T) error {
			return Value{}.Validate()
		},
		"replayed compile error": func(t *testing.T) error {
			bad, compileErr := Compile(`{status: =}`)
			require.Error(t, compileErr)
			return bad.Validate()
		},
		"replayed JSON compile error": func(t *testing.T) error {
			bad, compileErr := CompileJSON([]byte(`{not json`))
			require.Error(t, compileErr)
			return bad.Validate()
		},
		"cross-context derived bottom": func(t *testing.T) error {
			a, err := Compile(`{status: string}`)
			require.NoError(t, err)
			b, err := CompileJSON([]byte(`{"status": "✅"}`))
			require.NoError(t, err)
			c, err := Compile(`{weight: int}`)
			require.NoError(t, err)
			d, err := CompileJSON([]byte(`{"weight": 1}`))
			require.NoError(t, err)
			// Both operands are derived results in different contexts, so the
			// unification cannot rebuild either into the other and absorbs as a
			// pathless bottom.
			return c.Unify(d).Unify(a.Unify(b)).Validate()
		},
	}
	for name, build := range bottoms {
		t.Run(name, func(t *testing.T) {
			verr := build(t)
			require.Error(t, verr)
			// The invariant: Validate() != nil ⇒ len(Errors(verr)) ≥ 1.
			leaves := Errors(verr)
			require.GreaterOrEqual(t, len(leaves), 1,
				"a non-nil Validate error must decompose to at least one *PathError")
			assert.Empty(t, leaves[0].Path(),
				"a bottom-path error carries no specific leaf, so its path is empty")
		})
	}
}
