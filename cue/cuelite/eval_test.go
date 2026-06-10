package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckNoMisplacedDefault drives the static * default check across every
// AST position the walker descends, both where a misplaced mark must reject
// and where a valid default (or no mark) must pass. CUE rejects a misplaced
// mark at parse, even in an unreached position, so the in-house engine does
// the same up front.
func TestCheckNoMisplacedDefault(t *testing.T) {
	rejects := []string{
		`{a: *""}`,      // field value
		`{a: [*""][0]}`, // list element
		`{a: (*"")}`,    // parenthesized, non-disjunction
		`({mechanism:[if mechanism{},(*"")][0]})`, // unreached list element
		`{a: close({b: *1})}`,                     // call argument
		`{a: [if true {*""}][0]}`,                 // comprehension body field
		`{a: {b: *""}}`,                           // nested struct field
		`{a: [1, 2][*0]}`,                         // index expression target operand
		`{a: -(*"")}`,                             // nested unary operand
		`if true {a: *""}`,                        // top-level comprehension body
		`{a: [if true {b: *""}]}`,                 // comprehension as a list element
		`{a: [...(*"")]}`,                         // open-list tail element type
		`{a: [*""][0] | 2}`,                       // misplaced mark in a disjunction's left arm
		`{a: [*""][0] & int}`,                     // misplaced mark in a non-OR binary's left operand
		`{a: (*0) | 1}`,                           // a mark wrapped in its own parens is misplaced
		`{a: 1 | (*0)}`,                           // the same on the right disjunct
		`{a: ((*0)) | 1}`,                         // doubly parenthesized mark
	}
	for _, src := range rejects {
		_, err := Compile(src)
		assert.Error(t, err, "a misplaced * default must reject: %s", src)
	}
	// A top-level comprehension with no misplaced mark passes the default check
	// (it fails LATER as an unsupported top-level declaration, not on the mark),
	// exercising the visitDecl comprehension branch's success path.
	_, err := Compile(`if true {a: int}`)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "* default", "the * check must pass; a later rule rejects")
	accepts := []string{
		`{a: *1 | 2}`,           // simple default
		`{a: int | *"x"}`,       // default on the right
		`{a: *(1 | 2) | 3}`,     // default over a parenthesized disjunction
		`{a: (*1 | 2) | 3}`,     // a sub-disjunction's own direct mark stays valid
		`{a: *(0) | 1}`,         // a mark over a parenthesized single value is valid
		`{a: [1, 2][0]}`,        // list index, no mark
		`{a: close({b: int})}`,  // call, no mark
		`{a: {b: string}}`,      // nested struct, no mark
		`{a: [if true {1}][0]}`, // comprehension body, no mark
		`{a: [if true {b: 1}]}`, // comprehension list element, no mark
		`{a: [...int]}`,         // open-list tail with a type, no mark
		`{a: [1, 2, ...]}`,      // bare open-list tail (nil element type)
	}
	for _, src := range accepts {
		_, err := Compile(src)
		assert.NoError(t, err, "a valid default or no mark must compile: %s", src)
	}
}

// TestThunk_comparisons drives a deferred thunk whose `if` condition uses
// each comparison operator over a sibling reference, so evalComparison and
// compareConcrete cover the ordered, equality, and regex branches once the
// reference is fixed by data.
func TestThunk_comparisons(t *testing.T) {
	cases := []struct {
		name   string
		cond   string // the if-condition referencing sibling field n or s
		data   map[string]any
		reject bool // whether the body (a required non-empty string at registry) applies
	}{
		{"ge true", `n >= 3`, map[string]any{"n": int64(5), "r": ""}, true},
		{"ge false", `n >= 3`, map[string]any{"n": int64(1), "r": ""}, false},
		{"le true", `n <= 3`, map[string]any{"n": int64(1), "r": ""}, true},
		{"gt true", `n > 3`, map[string]any{"n": int64(5), "r": ""}, true},
		{"lt true", `n < 3`, map[string]any{"n": int64(1), "r": ""}, true},
		{"ne true", `n != 3`, map[string]any{"n": int64(1), "r": ""}, true},
		{"eq true", `n == 3`, map[string]any{"n": int64(3), "r": ""}, true},
		{"string ne", `s != "x"`, map[string]any{"n": int64(0), "s": "y", "r": ""}, true},
		{"regex match", `s =~ "^a"`, map[string]any{"n": int64(0), "s": "abc", "r": ""}, true},
		{"regex nonmatch", `s !~ "^a"`, map[string]any{"n": int64(0), "s": "bbc", "r": ""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// registry must be a non-empty string when the condition holds.
			schema := `{n: int, s?: string, r: [if ` + tc.cond + ` {string & != ""}, (string | *"")][0]}`
			v, err := Compile(schema)
			require.NoError(t, err)
			verr := v.CompileMap(tc.data).Validate()
			if tc.reject {
				assert.Error(t, verr, "empty r must reject when the condition holds")
			} else {
				assert.NoError(t, verr, "empty r is fine when the condition fails")
			}
		})
	}
}

// TestDeferrableList pins that a NON-indexed list field whose comprehension
// references a sibling defers to a thunk and resolves against data — CUE
// accepts `xs: [if c {1}, 2]`, so the in-house engine must too (a list literal
// is a deferrable construct, not a hard "reference not found"). An undeclared
// reference in the list still rejects at compile.
func TestDeferrableList(t *testing.T) {
	v, err := Compile(`{c: bool, xs: [if c {1}, 2]}`)
	require.NoError(t, err)
	// c=true keeps the if body, so xs is [1, 2]; c=false drops it, xs is [2].
	assert.NoError(t, v.CompileMap(map[string]any{"c": true, "xs": []any{int64(1), int64(2)}}).Validate())
	assert.NoError(t, v.CompileMap(map[string]any{"c": false, "xs": []any{int64(2)}}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"c": true, "xs": []any{int64(2)}}).Validate(),
		"c=true requires xs to start with 1")
	// An undeclared reference in the list is still a hard compile error.
	_, err = Compile(`{xs: [if undeclared {1}]}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `reference "undeclared" not found`)
}

// TestThunk_nestedListAndStruct drives a thunk whose evaluated body is a
// list and a struct, covering evalList and evalStruct's scoped builders.
func TestThunk_nestedListAndStruct(t *testing.T) {
	t.Run("thunk body is a struct", func(t *testing.T) {
		// When mode == "full", meta must be a struct with a non-empty title.
		schema := `{mode: string, meta: [if mode == "full" {{title: string & != ""}}, ({...} | *{})][0]}`
		v, err := Compile(schema)
		require.NoError(t, err)
		assert.Error(t, v.CompileMap(map[string]any{
			"mode": "full", "meta": map[string]any{"title": ""},
		}).Validate())
		assert.NoError(t, v.CompileMap(map[string]any{
			"mode": "full", "meta": map[string]any{"title": "ok"},
		}).Validate())
	})
	t.Run("thunk body is a list", func(t *testing.T) {
		// When mode == "list", tags must be a list of strings.
		schema := `{mode: string, tags: [if mode == "list" {[...string]}, (_ | *[])][0]}`
		v, err := Compile(schema)
		require.NoError(t, err)
		assert.NoError(t, v.CompileMap(map[string]any{
			"mode": "list", "tags": []any{"a", "b"},
		}).Validate())
		assert.Error(t, v.CompileMap(map[string]any{
			"mode": "list", "tags": []any{int64(1)},
		}).Validate())
	})
}

// TestThunk_unforcedIsIncomplete pins that a thunk validated without data
// (never forced) reports an incomplete leaf rather than silently passing.
func TestThunk_unforcedIsIncomplete(t *testing.T) {
	schema := `{mechanism: "push" | "pull", ` +
		`registry: [if mechanism == "push" {string & != ""}, (string | *"")][0]}`
	v, err := Compile(schema)
	require.NoError(t, err)
	// Validating the schema alone leaves the thunk unforced.
	assert.Error(t, v.Validate())
}

// TestThunk_indexOutOfRange covers the index-out-of-range ⊥ in evalIndex
// once the comprehension drops its only element.
func TestThunk_indexOutOfRange(t *testing.T) {
	// When cond is false, the comprehension contributes nothing, so [1] (and
	// here [0] of an otherwise-empty list) is out of range → ⊥.
	schema := `{c: bool, r: [if c {string}][0]}`
	v, err := Compile(schema)
	require.NoError(t, err)
	// c false → the list is empty → index 0 out of range → ⊥ at r.
	assert.Error(t, v.CompileMap(map[string]any{"c": false, "r": "x"}).Validate())
}

// TestThunk_forClauseRejected pins that a for-comprehension (outside the
// subset) is rejected with a clear message.
func TestThunk_forClauseRejected(t *testing.T) {
	_, err := Compile(`{xs: [...int], r: [for x in xs {x}][0]}`)
	require.Error(t, err)
}

// TestComprehension_deferredBodyHardError pins that a HARD error in a
// comprehension body is caught at compile even when the condition defers — CUE
// rejects the body's invalid operand regardless of whether the condition
// selects it. A body whose references merely defer still compiles.
func TestComprehension_deferredBodyHardError(t *testing.T) {
	t.Run("hard error under an unresolved-reference condition", func(t *testing.T) {
		// `mechanism` is unresolved, so the condition defers; the body
		// `{string != ""}` is an invalid operand that must reject at compile.
		_, err := Compile(`({mechanism:[if mechanism{string!=""}][0]})`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a concrete operand")
	})
	t.Run("hard error under a non-concrete-type condition", func(t *testing.T) {
		// `if string` is a non-concrete-type condition (it evaluates, not
		// defers as a reference), so it takes the non-concrete path; the body's
		// hard error must still reject.
		_, err := Compile(`{x: [if string {string != ""}][0]}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a concrete operand")
	})
	t.Run("a deferring body compiles when the condition defers", func(t *testing.T) {
		// The body `{string & != ""}` is a valid bound, not a hard error, so the
		// comprehension defers and resolves against data.
		v, err := Compile(`{m: string, x: [if m == "a" {string & != ""}][0]}`)
		require.NoError(t, err)
		assert.NoError(t, v.CompileMap(map[string]any{"m": "a", "x": "ok"}).Validate())
	})
}

// TestCompareConcrete_errors covers compareConcrete's error branches: a
// regex comparison against a non-string and a comparison of incomparable
// kinds, reached through a forced thunk.
func TestCompareConcrete_errors(t *testing.T) {
	t.Run("regex against non-string operand", func(t *testing.T) {
		// n is an int; `n =~ "x"` cannot compare → the thunk forces to ⊥.
		v, err := Compile(`{n: int, r: [if n =~ "x" {string}][0]}`)
		require.NoError(t, err)
		assert.Error(t, v.CompileMap(map[string]any{"n": int64(1), "r": "y"}).Validate())
	})
	t.Run("ordered comparison of bool", func(t *testing.T) {
		// b is a bool; `b >= 1` cannot compare numerically → ⊥.
		v, err := Compile(`{b: bool, r: [if b >= 1 {string}][0]}`)
		require.NoError(t, err)
		assert.Error(t, v.CompileMap(map[string]any{"b": true, "r": "y"}).Validate())
	})
}

// TestEvalComparison_typeOperand pins that a comparison with a non-concrete
// TYPE operand (`_`, `string`, an unresolved-to-type left or right side) is
// rejected at SCHEMA COMPILE rather than deferred to a thunk — matching CUE's
// eager "'>' requires concrete value". A type operand can never become
// concrete, so a deferred thunk could never resolve.
func TestEvalComparison_typeOperand(t *testing.T) {
	t.Run("right operand is top, left is an unresolved reference", func(t *testing.T) {
		// A > _: A is an unresolved sibling reference; _ is top. CUE rejects the
		// _ operand at compile, so the in-house engine must too (not defer A).
		_, err := Compile(`A: A > _`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a concrete operand")
	})
	t.Run("right operand is a type keyword", func(t *testing.T) {
		_, err := Compile(`{a: int, b: a == string}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a concrete operand")
	})
	t.Run("left operand is top", func(t *testing.T) {
		// _ > a: the LEFT operand is non-concrete; the right is an unresolved
		// reference. The left check fires first.
		_, err := Compile(`{a: int, b: _ > a}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a concrete operand")
	})
	t.Run("concrete-vs-reference still defers to a thunk", func(t *testing.T) {
		// a != "": both operands are valid (a is a deferred reference, "" is a
		// concrete string), so the comparison defers and resolves against data.
		v, err := Compile(`{a: string, xs: [a != ""]}`)
		require.NoError(t, err)
		assert.NoError(t, v.CompileMap(map[string]any{"a": "p", "xs": []any{true}}).Validate())
	})
	t.Run("a hard operand error propagates over a deferred reference", func(t *testing.T) {
		// `!0` is an unsupported construct (a hard error, not errUnresolved). It
		// can never resolve, so the comparison rejects at compile even when the
		// OTHER operand is an unresolved reference — in both operand positions.
		_, err := Compile(`{a: int, b: a > !0}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported unary operator "!"`)
		_, err = Compile(`{a: int, b: !0 > a}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported unary operator "!"`)
	})
}

// TestCompile_ifConditionNotBool pins that an `if` comprehension whose
// condition is not a concrete bool is rejected at SCHEMA COMPILE rather than
// crashing or deferring to a thunk that can never resolve — matching CUE's
// "cannot use ... as type bool". A concrete non-bool (`if ""`, `if 1`) and a
// non-concrete type/top condition (`if string`, `if _`) both reject; only a
// condition that resolves to a bool against data (`if m == "a"`) defers.
func TestCompile_ifConditionNotBool(t *testing.T) {
	for _, src := range []string{
		`({A: [if "" {}]})`,
		`({A: [if 1 {}]})`,
		`({A: [if string {}]})`,
		`({A: [if _ {}]})`,
	} {
		_, err := Compile(src)
		assert.Error(t, err, "a non-bool if condition must reject at compile: %s", src)
	}
	// A condition that resolves to a bool once data binds the reference defers
	// and validates — the deferral path is NOT broken by the rejection above.
	// With m == "a" the if body is kept, so [0] resolves to `string` and x must
	// be a string.
	v, err := Compile(`{m: string, x: [if m == "a" {string}][0]}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"m": "a", "x": "anything"}).Validate())
}

// TestLiftJSON_branches covers the JSON lifter's number, array, and scalar
// branches plus the strict-JSON rejections.
func TestLiftJSON_branches(t *testing.T) {
	t.Run("float number lifts", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"f": 1.5}`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("nested array of mixed scalars", func(t *testing.T) {
		v, err := CompileJSON([]byte(`[1, "x", true, null, [2]]`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid UTF-8 rejected", func(t *testing.T) {
		_, err := CompileJSON([]byte("\"a\xffb\""))
		require.Error(t, err)
	})
	t.Run("trailing data rejected", func(t *testing.T) {
		_, err := CompileJSON([]byte(`1 2`))
		require.Error(t, err)
	})
	t.Run("bare scalar document", func(t *testing.T) {
		v, err := CompileJSON([]byte(`true`))
		require.NoError(t, err)
		assert.NoError(t, v.Validate())
	})
}

// TestEvalIndex_int64Bounds pins the int64-space bounds check on list
// indexing: an index literal wider than 32 bits must reject as out of
// range on every platform rather than truncate on 32-bit targets and
// silently select a wrong-but-valid element.
func TestEvalIndex_int64Bounds(t *testing.T) {
	_, err := Compile(`close({ x: ["a", "b"][9223372036854775806] })`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}
