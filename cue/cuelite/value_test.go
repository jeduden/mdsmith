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
		// compile error as a path-free *PathError — preserving the message so a
		// caller that ignores the error still cannot mistake it for an accepting
		// value, while keeping the Errors invariant.
		assertBottomError(t, v.Validate(), err.Error())
	})
	t.Run("contradiction reduces to a bottom", func(t *testing.T) {
		// `int & string` has no satisfying value; Compile must surface the ⊥ as
		// an error so schema/extend.go's checkUnifiable sees the conflict.
		_, err := Compile(`{x: int & string}`)
		require.Error(t, err)
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
		// CUE accepts an unquoted key; strict JSON does not. CompileJSON must
		// reject it so CUE source cannot slip in through the data arm.
		_, err := CompileJSON([]byte(`{n: 3}`))
		require.Error(t, err)
	})
	t.Run("cue expression rejected as non-JSON", func(t *testing.T) {
		_, err := CompileJSON([]byte(`{"n": >=0}`))
		require.Error(t, err)
	})
	t.Run("conflicting duplicate key rejected", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a":1,"a":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
		assertBottomError(t, v.Validate(), err.Error())
	})
	t.Run("mergeable duplicate key rejected", func(t *testing.T) {
		// The strict-JSON contract forbids any duplicate key before the lift,
		// so a same-named key pair never merges into a phantom object.
		_, err := CompileJSON([]byte(`{"a":{"b":1},"a":{"c":2}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("equal duplicate key rejected", func(t *testing.T) {
		_, err := CompileJSON([]byte(`{"a":1,"a":1}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("nested duplicate key rejected", func(t *testing.T) {
		_, err := CompileJSON([]byte(`{"x":{"a":1,"a":1}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("duplicate key inside an array element rejected", func(t *testing.T) {
		_, err := CompileJSON([]byte(`[{"a":1,"a":1}]`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("same key in different objects accepted", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"x":{"a":1},"y":{"a":2}}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("a string value equal to its own key accepted", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a":"a"}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("a string value equal to an earlier key accepted", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"x":"y","y":1}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
}

// TestCompileJSON_edgeInputs covers the strict-JSON scanner's edge
// behavior: lossy-decode keys deferred to the decoder, a trailing second
// top-level value, an out-of-int64-range number lifting to a float, and a
// lone-surrogate value accepted as a concrete string (the in-house engine's
// own behavior — the former CUE lift rejected it).
func TestCompileJSON_edgeInputs(t *testing.T) {
	t.Run("invalid-UTF-8 raw keys are not fabricated duplicates", func(t *testing.T) {
		_, err := CompileJSON([]byte("{\"a\xff\":1,\"a\xfe\":2}"))
		if err != nil {
			assert.NotContains(t, err.Error(), "duplicate",
				"invalid UTF-8 must defer, not fabricate a duplicate")
		}
	})
	t.Run("lone-surrogate escaped keys are not duplicates of each other", func(t *testing.T) {
		// "\ud800" and "\udc00" decode to the same U+FFFD; they must not be
		// reported as duplicates of each other. The in-house lifter accepts the
		// resulting U+FFFD-keyed object as concrete data.
		v, err := CompileJSON([]byte(`{"\ud800":1,"\udc00":2}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("trailing second top-level value rejected", func(t *testing.T) {
		_, err := CompileJSON([]byte(`{"x":1} {"a":1,"a":2}`))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "duplicate",
			"the error must be trailing-data, not a fabricated scanner duplicate")
	})
	t.Run("duplicate beside an out-of-int64-range number rejected", func(t *testing.T) {
		_, err := CompileJSON([]byte(`{"x":1e999,"a":{"b":1},"a":{"c":2}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("out-of-int64-range number without duplicates accepted as float", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"x":1e999}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("lone-surrogate value accepted as a concrete string", func(t *testing.T) {
		// The in-house lifter decodes "\ud800" to U+FFFD and accepts it as a
		// concrete string. The differential oracle is updated in lockstep.
		v, err := CompileJSON([]byte(`{"a": "\ud800"}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
}

func TestValue_Unify(t *testing.T) {
	t.Run("merges two values", func(t *testing.T) {
		schema, err := Compile(`{status: string}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		assert.NoError(t, schema.Unify(data).Validate())
	})
	t.Run("operand order does not matter", func(t *testing.T) {
		// A context-free Value unifies the same either way; the shared schema
		// can be receiver or operand.
		schema, err := Compile(`{status: string}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		assert.NoError(t, schema.Unify(data).Validate())
		assert.NoError(t, data.Unify(schema).Validate())
	})
	t.Run("bottom receiver absorbs", func(t *testing.T) {
		bad, compileErr := Compile(`{status: =}`)
		require.Error(t, compileErr)
		ok, err := Compile(`{status: string}`)
		require.NoError(t, err)
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
		assert.NoError(t, ok.Validate(), "concrete receiver alone must pass")
		assertBottomError(t, ok.Unify(Value{}).Validate(), errZeroValue.Error())
	})
	t.Run("zero receiver against concrete operand absorbs", func(t *testing.T) {
		ok, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		assertBottomError(t, Value{}.Unify(ok).Validate(), errZeroValue.Error())
	})
}

// TestValue_Unify_chained exercises chaining a derived Unify result against
// further values: constraints accumulate, and a conflicting later value is
// rejected.
func TestValue_Unify_chained(t *testing.T) {
	t.Run("chained unify against a derived result keeps constraints", func(t *testing.T) {
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
	t.Run("derived result re-unified keeps a non-concrete leaf", func(t *testing.T) {
		a, err := Compile(`{status: string, weight: int}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		ab := a.Unify(b)
		merged := a.Unify(ab)
		assert.Error(t, merged.Validate(), "weight still non-concrete")
	})
}

// TestValue_Unify_singleContext pins the post-flip contract that replaced
// the interim cross-context-bottom behavior: a chained unify of derived
// values SUCCEEDS and validates per single-context CUE semantics, because a
// context-free Value has no contexts to cross (plan 238).
func TestValue_Unify_singleContext(t *testing.T) {
	t.Run("compatible roots validate regardless of chaining order", func(t *testing.T) {
		schema, err := Compile(`{status: string}`)
		require.NoError(t, err)
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		assert.NoError(t, schema.Unify(a.Unify(data)).Validate())
	})
	t.Run("conflicting roots reject with the field path", func(t *testing.T) {
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
	t.Run("two derived results unify and keep both constraints", func(t *testing.T) {
		// The former interim absorbed a.Unify(b).Unify(c.Unify(d)) as a pathless
		// bottom across contexts. The flip restores single-context CUE: the two
		// derived results unify; the int field stays non-concrete, so Validate
		// rejects at weight — the value composed, it did not absorb.
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		c, err := Compile(`{weight: int}`)
		require.NoError(t, err)
		d, err := Compile(`{height: int}`)
		require.NoError(t, err)
		verr := a.Unify(b).Unify(c.Unify(d)).Validate()
		require.Error(t, verr)
		leaves := Errors(verr)
		paths := make([][]string, 0, len(leaves))
		for _, l := range leaves {
			paths = append(paths, l.Path())
		}
		assert.Contains(t, paths, []string{"weight"})
		assert.Contains(t, paths, []string{"height"})
	})
	t.Run("two compatible derived data results unify and accept", func(t *testing.T) {
		a, err := Compile(`{status: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"status": "✅"}`))
		require.NoError(t, err)
		c, err := Compile(`{weight: int}`)
		require.NoError(t, err)
		d, err := CompileJSON([]byte(`{"weight": 1}`))
		require.NoError(t, err)
		assert.NoError(t, a.Unify(b).Unify(c.Unify(d)).Validate())
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
		// The path must appear exactly once. The message is the in-house
		// engine's own stable wording (plan 238): conflicting values, lowercase.
		assert.Equal(
			t,
			`meta.status: conflicting values "✅" and "🔲"`,
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

// TestValue_CompileMap pins the direct map-validation hot path: a
// map[string]any validates against a compiled schema with no JSON
// round-trip.
func TestValue_CompileMap(t *testing.T) {
	t.Run("satisfying map passes", func(t *testing.T) {
		schema, err := Compile(`close({status: string, weight: int})`)
		require.NoError(t, err)
		got := schema.CompileMap(map[string]any{"status": "done", "weight": 3})
		assert.NoError(t, got.Validate())
	})
	t.Run("conflicting map fails at the leaf path", func(t *testing.T) {
		schema, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		got := schema.CompileMap(map[string]any{"status": "🔲"})
		verr := got.Validate()
		require.Error(t, verr)
		leaves := Errors(verr)
		require.Len(t, leaves, 1)
		assert.Equal(t, []string{"status"}, leaves[0].Path())
	})
	t.Run("a YAML int satisfies int and a YAML float satisfies float", func(t *testing.T) {
		// A YAML/JSON decoder hands a whole number back as an int and a decimal
		// as a float64, and the lifter preserves that kind, matching CUE: 42
		// satisfies int, 42.0 satisfies float, and a float64 NEVER coerces to int
		// (CUE keeps 42 and 42.0 distinct — the JSON lift of `42.0` against `int`
		// is a conflict).
		schemaInt, err := Compile(`{weight: int}`)
		require.NoError(t, err)
		assert.NoError(t, schemaInt.CompileMap(map[string]any{"weight": 42}).Validate())
		schemaFloat, err := Compile(`{weight: float}`)
		require.NoError(t, err)
		assert.NoError(t, schemaFloat.CompileMap(map[string]any{"weight": float64(42)}).Validate())
		// A float64 carrying a whole number is still a float, so it conflicts with
		// int — the CUE-correct behavior the differential oracle agrees with.
		assert.Error(t, schemaInt.CompileMap(map[string]any{"weight": float64(42)}).Validate())
	})
	t.Run("unsupported value type yields a bottom", func(t *testing.T) {
		schema, err := Compile(`{t: _}`)
		require.NoError(t, err)
		got := schema.CompileMap(map[string]any{"t": struct{}{}})
		assert.Error(t, got.Validate())
	})
	t.Run("bottom receiver absorbs", func(t *testing.T) {
		bad, _ := Compile(`{x: =}`)
		assert.Error(t, bad.CompileMap(map[string]any{"x": 1}).Validate())
	})
}

// TestValidate_unwrapsBottom asserts the bottom path keeps the original
// error reachable through errors.Is: a PathError built for a bottom Value
// wraps the bottom's cause, so a caller can test for a sentinel (here
// errZeroValue) through the returned validation error.
func TestValidate_unwrapsBottom(t *testing.T) {
	verr := Value{}.Validate()
	require.Error(t, verr)
	assert.True(t, stderrors.Is(verr, errZeroValue),
		"a bottom PathError must unwrap to its underlying cause")
}

// TestValidate_invariant pins the contract every consumer loop relies on: a
// non-nil Validate error always decomposes to at least one *PathError, so a
// loop over Errors never emits zero diagnostics for a failing value.
func TestValidate_invariant(t *testing.T) {
	schema, err := Compile(`{status: string}`)
	require.NoError(t, err)
	// A non-concrete schema (status awaits data) is a failing value.
	verr := schema.Validate()
	require.Error(t, verr)
	leaves := Errors(verr)
	require.GreaterOrEqual(t, len(leaves), 1,
		"a non-nil Validate error must decompose to at least one *PathError")
	assert.Equal(t, []string{"status"}, leaves[0].Path())
}
