package cuelite

import (
	stderrors "errors"
	"testing"

	cueerrors "cuelang.org/go/cue/errors"
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
	t.Run("conflicting duplicate key rejected", func(t *testing.T) {
		// {"a":1,"a":2} would extract as JSON and unify to a conflicting
		// bottom; strict JSON rejects any duplicate key before the CUE lift,
		// so CompileJSON surfaces a Go error naming the key.
		v, err := CompileJSON([]byte(`{"a":1,"a":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
		assertBottomError(t, v.Validate(), err.Error())
	})
	t.Run("mergeable duplicate key rejected", func(t *testing.T) {
		// CUE UNIFIES duplicate object keys, so {"a":{"b":1},"a":{"c":2}}
		// would compile to a phantom merged object. Real JSON consumers are
		// last-wins; strict JSON forbids any duplicate key. CompileJSON must
		// reject it before the CUE lift.
		_, err := CompileJSON([]byte(`{"a":{"b":1},"a":{"c":2}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("equal duplicate key rejected", func(t *testing.T) {
		// CUE accepts {"a":1,"a":1} (equal unifies to itself); strict JSON
		// still forbids the duplicate.
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
		// The same name in two SEPARATE objects is not a duplicate.
		v, err := CompileJSON([]byte(`{"x":{"a":1},"y":{"a":2}}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("a string value equal to its own key accepted", func(t *testing.T) {
		// {"a":"a"}: the value "a" matches the key "a", but a value is not a
		// key, so the seenKey parity must not treat it as a duplicate. Deleting
		// the !cur.seenKey parity guard would misread the value as a key and
		// fabricate a duplicate.
		v, err := CompileJSON([]byte(`{"a":"a"}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("a string value equal to an earlier key accepted", func(t *testing.T) {
		// {"x":"y","y":1}: the value "y" of the first pair equals the second
		// key "y". Without the parity guard the value would be tracked as a
		// key and collide with the real "y" key.
		v, err := CompileJSON([]byte(`{"x":"y","y":1}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
}

// TestCompileJSON_edgeInputs covers the strict-JSON scanner's edge
// behavior: lossy-decode keys deferred to Extract, a trailing second
// top-level value, an out-of-float64-range number, and a lone-surrogate
// value surfaced as a data-stage compile error.
func TestCompileJSON_edgeInputs(t *testing.T) {
	t.Run("invalid-UTF-8 raw keys are not fabricated duplicates", func(t *testing.T) {
		// json.Decoder replaces each invalid byte in a raw key with U+FFFD, so
		// two distinct invalid-byte keys would collapse to one fabricated
		// "duplicate JSON key". A utf8.Valid pre-check defers such input to
		// Extract, so the error (if any) is Extract's, not a fabricated dup.
		_, err := CompileJSON([]byte("{\"a\xff\":1,\"a\xfe\":2}"))
		if err != nil {
			assert.NotContains(t, err.Error(), "duplicate",
				"invalid UTF-8 must defer to Extract, not fabricate a duplicate")
		}
	})
	t.Run("lone-surrogate escaped keys are not duplicates of each other", func(t *testing.T) {
		// "\ud800" and "\udc00" are distinct lone-surrogate escapes whose
		// decoded keys both become U+FFFD. They must NOT be reported as
		// duplicates of each other; the scanner skips dup tracking for any
		// decoded key containing U+FFFD. (Each still errors at the build via
		// the restored val.Err() guard, so CompileJSON still rejects — but for
		// the surrogate reason, not a fabricated duplicate.)
		_, err := CompileJSON([]byte(`{"\ud800":1,"\udc00":2}`))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "duplicate",
			"lone-surrogate keys must not be fabricated duplicates")
	})
	t.Run("trailing second top-level value defers to Extract", func(t *testing.T) {
		// {"x":1} {"a":1,"a":2}: the scanner must stop once the first
		// top-level value closes, leaving the trailing data to Extract — which
		// reports "invalid JSON ... after top-level value" — rather than
		// fabricating a duplicate "a" from a second top-level object Extract
		// never reaches.
		_, err := CompileJSON([]byte(`{"x":1} {"a":1,"a":2}`))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "duplicate",
			"the error must come from Extract, not a fabricated scanner duplicate")
	})
	t.Run("duplicate beside an out-of-float64-range number rejected", func(t *testing.T) {
		// 1e999 is valid JSON but overflows float64; without dec.UseNumber()
		// json.Decoder.Token errors on it mid-scan, the scanner misreads that
		// as malformed and defers, and the mergeable duplicate "a" slips past
		// into a phantom merged object. The scanner must keep scanning and
		// reject the duplicate.
		_, err := CompileJSON([]byte(`{"x":1e999,"a":{"b":1},"a":{"c":2}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("out-of-float64-range number without duplicates accepted", func(t *testing.T) {
		// The same overflowing number, with no duplicate key, must still
		// compile end-to-end: the scanner defers cleanly and Extract lifts it.
		v, err := CompileJSON([]byte(`{"x":1e999}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("lone-surrogate value rejected at the data stage", func(t *testing.T) {
		// A lone-surrogate escape such as "\ud800" is grammar-valid
		// duplicate-free strict JSON, so it passes the scanner and
		// cuejson.Extract, but ctx.BuildExpr yields a bottom ("invalid
		// string: unmatched surrogate pair"). buildJSON must surface that
		// bottom as a compile error rather than returning an accepting Value,
		// so the data arm classifies it at the data stage.
		v, err := CompileJSON([]byte(`{"a": "\ud800"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "surrogate")
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
		// bearing: removing it lets Unify reach o.rebuild on a zero operand,
		// whose o.val.Context() is nil, so the (*cue.Context)(nil).
		// CompileString call panics with a nil-pointer dereference — this
		// assertion's errZeroValue reason can only come from the guard.
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

// TestValue_Unify_crossContext covers order-insensitive cross-context
// unification: a derived operand against a source-carrying receiver
// rebuilds the RECEIVER into the operand's context, so c.Unify(
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

// TestValidate_unwrapsToCueError asserts a per-leaf PathError from a real
// validation conflict keeps the original cue/errors error reachable
// through errors.As, so a caller is not cut off from the CUE error it
// wraps. The leaf is produced by pathErrorOf on the bottom-free path.
func TestValidate_unwrapsToCueError(t *testing.T) {
	schema, err := Compile(`{status: "✅"}`)
	require.NoError(t, err)
	data, err := CompileJSON([]byte(`{"status": "🔲"}`))
	require.NoError(t, err)
	verr := schema.Unify(data).Validate()
	require.Error(t, verr)
	var cueErr cueerrors.Error
	assert.True(t, stderrors.As(verr, &cueErr),
		"a pathErrorOf leaf must unwrap to the cue/errors error it wraps")
}

// TestValidate_invariant pins the contract every consumer loop relies on:
// a non-nil Validate error always decomposes to at least one *PathError,
// so a loop over Errors never emits zero diagnostics for a failing value.
// The bottom flavors that were the gap — zero Value, a replayed compile
// error, a replayed JSON compile error — are each pinned through
// assertBottomError elsewhere (TestValue_Validate, TestCompile,
// TestCompileJSON). The one shape not message-pinned anywhere else is the
// SWAPPED-order cross-context derived bottom: c.Unify(d).Unify(a.Unify(b)),
// the mirror of TestValue_Unify_crossContext's a.Unify(b).Unify(c.Unify(d)).
// It must still surface one empty-path *PathError, not a bare Go error
// Errors would flatten to nil.
func TestValidate_invariant(t *testing.T) {
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
	verr := c.Unify(d).Unify(a.Unify(b)).Validate()
	require.Error(t, verr)
	// The invariant: Validate() != nil ⇒ len(Errors(verr)) ≥ 1.
	leaves := Errors(verr)
	require.GreaterOrEqual(t, len(leaves), 1,
		"a non-nil Validate error must decompose to at least one *PathError")
	assert.Empty(t, leaves[0].Path(),
		"a bottom-path error carries no specific leaf, so its path is empty")
}
