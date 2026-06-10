package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultPairFlatCancellation pins CUE's ⟨value, default⟩ model for a FLAT
// disjunction whose marked default value also appears as an unmarked sibling:
// the default survives regardless of branch order, so the absent field takes
// it. Probed against CUE v0.16.1: every case accepts. The pre-redesign engine
// dropped the marked pointer at concrete-dedup (first occurrence won), so a
// `*0` after a plain `0` was lost.
func TestDefaultPairFlatCancellation(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"mark middle", `{A: 0|*0|1}`, `{}`, true},
		{"mark last", `{A: 1|0|*0}`, `{}`, true},
		{"mark first", `{A: *0|0|1}`, `{}`, true},
		{"nested right equal-dup", `{A: 0|(*0|1)}`, `{}`, true},
		{"string nested dup", `{A: "x"|(*"x"|"y")}`, `{}`, true},
		{"nested meet int", `{A: (0|(*0|1))&int}`, `{}`, true},
		{"flat meet int", `{A: (0|*0|1)&int}`, `{}`, true},
	})
}

// TestDefaultPairNestedCollapse pins the round-2 carry: a PARENTHESIZED nested
// disjunction whose VALUE collapses to a single branch (`*0|0` reduces to the
// value 0) carries NO default up to the outer disjunction — a single-value
// disjunction is not a disjunction, so its default is discarded. Probed against
// CUE v0.16.1: each rejects an absent field, while a nested disjunction whose
// value stays multi-branch (`*0|1`) keeps its default and accepts.
func TestDefaultPairNestedCollapse(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"int dup collapses", `{A:(*0|0)|10}`, `{}`, false},
		{"int dup left collapses", `{A:(0|*0)|1}`, `{}`, false},
		{"string dup collapses", `{A:(*"x"|"x")|"y"}`, `{}`, false},
		{"bool dup collapses", `{A:(*true|true)|false}`, `{}`, false},
		{"plain nested no default", `{A:(0|0)|10}`, `{}`, false},
		// The non-duplicate nested forms still keep the default.
		{"nested non-dup keeps default", `{A:(*0|1)|10}`, `{}`, true},
		{"nested non-dup left keeps default", `{A:(*0|1)|0}`, `{}`, true},
		{"nested multi keeps default", `{A:(*0|1|2)|10}`, `{}`, true},
		{"nested meet keeps default", `{A:((*0|1)|10) & number}`, `{}`, true},
		{"nested collapse meet drops default", `{A:((*0|0)|10) & number}`, `{}`, false},
	})
}

// TestMeetBranchesNestedBottomPruning pins that a struct/list/bound/defaulted
// branch whose meet with the data produced a NESTED bottom (a closed-struct
// violation, a field conflict, a bound failure, anywhere in the meet) is pruned
// like a top-level bottom, so the surviving branch decides the disjunction.
// Probed against CUE v0.16.1: every case accepts. The pre-redesign engine only
// dropped a branch whose meet was a TOP-LEVEL bottom, so a struct branch with a
// nested conflicting field survived and left the disjunction non-concrete.
func TestMeetBranchesNestedBottomPruning(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"closed struct branches", `{a: close({x:int}) | close({y:string})}`, `{"a":{"x":1}}`, true},
		{"open struct branches", `{a:{x:1}|{x:2}}`, `{"a":{"x":2}}`, true},
		{"list branches", `{a: [1,2] | [3,4]}`, `{"a":[3,4]}`, true},
		{"bound branches", `{a: (>=0 & <=5) | (>=10 & <=15)}`, `{"a":12}`, true},
		{"defaulted struct branches", `{a: {x:1} | *{x:2}}`, `{"a":{"x":1}}`, true},
		// A nested-struct branch with a deeper conflicting field still prunes.
		{"deep nested struct branch", `{a: {x:{y:1}} | {x:{y:2}}}`, `{"a":{"x":{"y":2}}}`, true},
	})
}

// TestFixpointThunkForcing pins that an acyclic chain of thunk fields, each
// referencing a sibling resolved by an EARLIER thunk's force, resolves through
// iterated force passes. Probed against CUE v0.16.1: accepts (n=1, o=3). The
// pre-redesign engine forced each thunk once against the initial concrete
// scope, so a thunk depending on another thunk's result never resolved.
func TestFixpointThunkForcing(t *testing.T) {
	const chain = `{m: string, n: [if m == "p" {1}, 2][0], o: [if n == 1 {3}, 4][0]}`
	runAcceptCases(t, []acceptCase{
		{"two-step chain", chain, `{"m":"p"}`, true},
		{"two-step chain other branch", chain, `{"m":"q"}`, true},
	})
}

// TestThunkSiblingDefaultResolution pins evalIdent's default resolution: a
// sibling whose value is a DEFAULTED disjunction resolves to its DEFAULT for a
// thunk's comparison. Probed against CUE v0.16.1: with `m: string | *"p"`, the
// thunk reads m == "p" as true, so n resolves to 1. A NON-defaulted disjunction
// sibling stays non-concrete and CUE rejects at the sibling (incomplete).
func TestThunkSiblingDefaultResolution(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"defaulted sibling resolves to default", `{m: string | *"p", n: [if m == "p" {1}, 2][0]}`, `{"n":1}`, true},
		{"defaulted sibling default rejects wrong", `{m: string | *"p", n: [if m == "p" {1}, 2][0]}`, `{"n":2}`, false},
	})
	// A non-defaulted disjunction sibling is incomplete: CUE rejects at m.
	t.Run("non-defaulted sibling incomplete", func(t *testing.T) {
		s, err := Compile(`{m: string | "p", n: [if m == "p" {1}, 2][0]}`)
		require.NoError(t, err)
		d, err := CompileJSON([]byte(`{"n":1}`))
		require.NoError(t, err)
		assert.Error(t, s.Unify(d).Validate())
	})
}

// TestMeetDefaultReconciliation pins CUE's default-of-a-meet rule across the
// cross-product cases the fuzzer probed: when both operand defaults survive the
// meet their reconciliation is the default (`(*0|int)&(0|*int)` → 0&int = 0);
// when only one survives it is that one (`(*1|2|9)&(*2|3|9)` → 2); two
// build-time marks stay ambiguous (`*string | *""` rejects). Probed against
// cuelang v0.16.1.
func TestMeetDefaultReconciliation(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"both defaults survive reconcile to 0", `{A:(*0|int)&(0|*int)}`, `{}`, true},
		{"one default survives", `{A:(*1|2|9)&(*2|3|9)}`, `{}`, true},
		{"one default survives two-branch", `{A:(*1|2)&(*2|3)}`, `{}`, true},
		{"conflicting marked defaults reject", `{A:(*1|int)&(*2|int)}`, `{}`, false},
		{"int meet picks default", `{A:int & (*1|2)}`, `{}`, true},
		{"build-time double mark ambiguous", `{A: *string | *""}`, `{}`, false},
		{"build-time two int marks ambiguous", `{A: *0 | *1}`, `{}`, false},
	})
}

// TestConcreteNonScalarDefault pins that a concrete LIST or STRUCT default
// (`*[]`, `*{x:0}`) works like a scalar default: it deduplicates against an
// equal sibling branch, resolves the meet default, and matches a provided
// equal value. The `depends-on: [...int] | *[]` schema the plan/ proto uses is
// the live case — both an ABSENT and a PROVIDED empty list must accept. Probed
// against cuelang v0.16.1.
func TestConcreteNonScalarDefault(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"list default absent", `{A: [...int] | *[]}`, `{}`, true},
		{"list default provided empty", `{A: [...int] | *[]}`, `{"A":[]}`, true},
		{"list default provided non-empty", `{A: [...int] | *[]}`, `{"A":[1,2]}`, true},
		{"struct default absent", `{A: {x:int} | *{x:0}}`, `{}`, true},
		{"list mark left absent", `{A: *[1,2] | [3,4]}`, `{}`, true},
		{"two list marks ambiguous", `{A: *[1] | *[2]}`, `{}`, false},
		{"equal list marks collapse", `{A: *[1] | *[1]}`, `{}`, true},
	})
}

// TestIndexNonListTarget pins that indexing a non-list TARGET (`0[mech]`,
// `"s"[0]`) is rejected at schema COMPILE — even when the index is an
// unresolved reference, since the target being a non-list is a type error
// regardless of the index. CUE rejects "invalid operand … want list or
// struct"; the in-house engine must not defer a thunk that rejects at validate.
// A valid list index by a sibling reference still resolves. Probed against
// cuelang v0.16.1.
func TestIndexNonListTarget(t *testing.T) {
	for _, src := range []string{
		`{mech: string, A: 0[mech]}`,
		`{A: 0[1]}`,
		`{mech: string, A: "s"[mech]}`,
	} {
		_, err := Compile(src)
		require.Error(t, err, src)
		assert.Contains(t, err.Error(), "invalid operation", src)
	}
	// A valid list index by a sibling reference resolves against data.
	s, err := Compile(`{A: [1, 2][mech], mech: int}`)
	require.NoError(t, err)
	d, err := CompileJSON([]byte(`{"mech":0}`))
	require.NoError(t, err)
	assert.NoError(t, s.Unify(d).Validate())
}

// TestMeetThunkRefErasure pins that an undeclared reference inside a deferred
// branch of a compile-time `&` meet surfaces as "reference not found" at
// compile, not silently erased by the eager meet. Probed against CUE v0.16.1:
// `{(int)&(0<A|int)}` is a schema compile error "reference A not found".
func TestMeetThunkRefErasure(t *testing.T) {
	_, err := Compile(`{(int)&(0<A|int)}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestOrderedComparisonNonOrderable pins that an ordered comparison (>, >=, <,
// <=) on a concrete BOOL or NULL operand is rejected at schema COMPILE, even
// when the other operand is an unresolved reference. A chained `0 > 0 > A`
// parses as `(0>0) > A` = `false > A`, which CUE rejects as "invalid operands";
// the in-house engine must reject it eagerly, not defer a thunk that rejects at
// validate. Probed against cuelang v0.16.1. A == / != on a bool is still fine,
// and a string IS orderable.
func TestOrderedComparisonNonOrderable(t *testing.T) {
	reject := []string{
		`{B: 0 > 0 > A, A: 0}`,
		`{B: false > A, A: 0}`,
		`{B: true > 1}`,
		`{B: null < A, A: 0}`,
		`{B: A < false, A: 0}`, // RIGHT operand non-orderable, left unresolved
		// A chained comparison whose inner comparison DEFERS (`0 > A`): the inner
		// is bool-typed regardless of A, so the outer ordered op is invalid even
		// though A is unresolved at compile.
		`{B: 0 > A > 0, A: 0}`,
		`{B: (0 > A) > 0, A: 0}`,
		`{B: A > (0 > 1), A: 0}`,         // RIGHT operand is a comparison expr
		`{m: string, B: (m =~ "x") > 0}`, // a regex-match operand is bool-typed
		`{m: string, B: (m !~ "x") < 0}`, // a regex-non-match operand is bool-typed
	}
	for _, src := range reject {
		_, err := Compile(src)
		require.Error(t, err, "must reject ordered compare on non-orderable: %s", src)
		assert.Contains(t, err.Error(), "invalid operation", src)
	}
	accept := []string{
		`{B: false == A, A: 0}`, // == admits any concrete
		`{B: "a" < "b"}`,        // strings are orderable
		`{B: 1 > 0}`,
	}
	for _, src := range accept {
		_, err := Compile(src)
		require.NoError(t, err, "must accept orderable / equality compare: %s", src)
	}
}
