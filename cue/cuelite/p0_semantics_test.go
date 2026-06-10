package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acceptCase is one (schema, JSON data) pair plus the expected accept/reject
// verdict, the minimal shape for pinning an engine-semantics decision. A nil
// data string means "validate the schema alone" (the absent-field default
// path).
type acceptCase struct {
	name   string
	schema string
	data   string // "" → validate schema alone
	accept bool   // expected Validate() == nil
}

// runAcceptCases compiles each schema, optionally unifies JSON data, and
// asserts the accept/reject verdict. A schema that must reject at COMPILE time
// (a contradiction the compiler reduces to ⊥) is allowed to surface its
// rejection as a compile error rather than a validate error — both are
// "reject".
func runAcceptCases(t *testing.T, cases []acceptCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := Compile(c.schema)
			if err != nil {
				assert.False(t, c.accept, "schema rejected at compile: %v", err)
				return
			}
			u := s
			if c.data != "" {
				d, derr := CompileJSON([]byte(c.data))
				require.NoError(t, derr)
				u = s.Unify(d)
			}
			got := u.Validate() == nil
			assert.Equal(t, c.accept, got, "verdict (verr=%v)", u.Validate())
		})
	}
}

// TestDisjunctionDefaults pins CUE's default-of-meet semantics across the
// three related bugs probed against CUE v0.16.1: multiple * marks in one
// disjunction are ambiguous (non-concrete), the default of a meet is the meet
// of the defaults, and a parenthesized nested default is carried up the flatten.
func TestDisjunctionDefaults(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		// 1a. Multiple * marks in ONE disjunction are ambiguous: CUE reduces
		// `*string | *""` to the value `string | ""` with no usable default, so
		// the absent field stays non-concrete and rejects.
		{"multiple marks reject", `{A: *string | *""}`, `{}`, false},
		{"two int marks reject", `{x: *1 | *2}`, `{}`, false},
		// Two EQUAL marked disjuncts collapse to one default.
		{"equal marks accept", `{x: *1 | *1}`, `{}`, true},
		// 1b. Default of a meet is the meet of the defaults. 1 & 2 = ⊥, so no
		// default survives and the field is non-concrete.
		{"meet of defaults conflicts", `{x: (*1 | int) & (*2 | int)}`, `{}`, false},
		{"meet of equal defaults", `{x: (*1 | int) & (*1 | int)}`, `{}`, true},
		// A default met against a default-LESS disjunction whose surviving meet
		// stays a disjunction (`int & (*1|int)` survives as `1|int`): the default
		// 1 lives inside that survivor, so it must be retained regardless of
		// operand order. (Regression: retainByValue only matched bare-scalar
		// survivors, dropping the default and leaving the field non-concrete.)
		{"default met vs defaultless, mark right", `{x: (0 | int) & (*1 | int)}`, `{}`, true},
		{"default met vs defaultless, mark left", `{x: (*1 | int) & (0 | int)}`, `{}`, true},
		{"meet default vs branches", `{x: (*1 | 2) & (2 | 3)}`, `{}`, true},
		{"meet default vs default empty", `{x: (*1 | 2) & (1 | *2)}`, `{}`, false},
		{"meet of two-mark defaults survives one", `{x: (*1 | *2) & (*2 | *3)}`, `{}`, true},
		{"meet of two-mark defaults ambiguous", `{x: (*1 | *2) & (*1 | *2)}`, `{}`, false},
		// 1c. A parenthesized nested disjunction carries its inner default up.
		{"nested default left", `{x: (*1 | 2) | 3}`, `{}`, true},
		{"flat default", `{x: *1 | 2 | 3}`, `{}`, true},
		{"nested default right wins ambiguous", `{x: (*1 | 2) | *3}`, `{}`, false},
		{"nested default on right branch", `{x: 1 | (*2 | 3)}`, `{}`, true},
		// A * on a parenthesized disjunction marks every inner disjunct.
		{"star over paren is ambiguous", `{x: *(1 | 2) | 3}`, `{}`, false},
	})
}

// TestEqualConcreteDisjunctsCollapse pins that equal concrete disjuncts
// collapse at BUILD time, so `"x" | "x"` is the concrete "x" and accepts the
// absent-field default — CUE de-duplicates equal disjuncts.
func TestEqualConcreteDisjunctsCollapse(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"equal string disjuncts", `{A: "x" | "x"}`, `{}`, true},
		{"equal int disjuncts", `{x: 1 | 1}`, `{}`, true},
		{"distinct disjuncts stay non-concrete", `{x: 1 | 2}`, `{}`, false},
	})
}

// TestAllBottomDisjunctionIsCompileError pins that a disjunction whose every
// disjunct reduces to ⊥ is a COMPILE error ("empty disjunction"), matching
// CUE's "errors in empty disjunction" at CompileSchema, rather than deferring
// to validate.
func TestAllBottomDisjunctionIsCompileError(t *testing.T) {
	_, err := Compile(`{x: 0&1 | 1&0}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty disjunction")
	// A disjunction with ONE live branch keeps it: `0&1 | 2` is the concrete 2.
	v, err := Compile(`{x: 0&1 | 2}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"x": 2}).Validate())
}

// TestBoundIntersectionConflict pins that an empty numeric/string bound
// interval reduces to ⊥ at COMPILE time, the behavior schema/extend.go's
// checkUnifiable depends on. CUE rejects these as "incompatible number/string
// bounds" at CompileSchema. != , =~ , !~ are NOT folded into the check
// (matching CUE), so `>=5 & <=5 & !=5` compiles even though it is empty.
func TestBoundIntersectionConflict(t *testing.T) {
	reject := []string{
		`{x: >=10 & <=5}`,
		`{x: >0 & <0}`,
		`{x: >5 & <5}`,
		`{x: >=10 & <10}`,
		`{x: >10 & <=10}`,
		`{x: >="b" & <="a"}`,
	}
	for _, src := range reject {
		_, err := Compile(src)
		assert.Error(t, err, "must reject empty bound interval: %s", src)
	}
	accept := []string{
		`{x: >=5 & <=10}`,
		`{x: >=5 & <=5}`,       // singleton {5}, non-empty
		`{x: >=5 & <=5 & !=5}`, // != not folded — compiles
		`{x: >5 & >10}`,        // same direction, never conflicts
		`{x: =~"a" & !~"b"}`,
	}
	for _, src := range accept {
		_, err := Compile(src)
		assert.NoError(t, err, "must accept satisfiable bounds: %s", src)
	}
}

// TestNumericComparisonAcrossKinds pins that the relational == / != operators
// compare numbers by VALUE across int and float (2 == 2.0 is true), matching
// CUE — distinct from the lattice meet, which keeps int and float disjoint.
func TestNumericComparisonAcrossKinds(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want bool
	}{
		{"int eq float", `x: 2 == 2.0`, true},
		{"int ne float false", `x: 2 != 2.0`, false},
		{"int eq int", `x: 2 == 2`, true},
		{"unequal numbers", `x: 1 == 1.5`, false},
		{"string eq", `x: "a" == "a"`, true},
		{"bool eq", `x: true == true`, true},
		{"null eq", `x: null == null`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := compileSource(c.expr)
			require.NoError(t, err)
			f := v.fields[0].val
			require.Equal(t, kBool, f.kind)
			assert.Equal(t, c.want, f.b)
		})
	}
}

// TestFloatLiftMatchesCUE pins that a float64 always lifts to a float leaf —
// no integral coercion — so the two lift paths (CompileMap, CompileJSON) agree
// with each other and with CUE: 42.0 satisfies float, 42 satisfies int, and a
// whole-number float64 NEVER coerces to int.
func TestFloatLiftMatchesCUE(t *testing.T) {
	t.Run("CompileMap", func(t *testing.T) {
		sFloat, err := Compile(`{x: float}`)
		require.NoError(t, err)
		assert.NoError(t, sFloat.CompileMap(map[string]any{"x": float64(2)}).Validate())
		assert.NoError(t, sFloat.CompileMap(map[string]any{"x": 2.5}).Validate())
		sInt, err := Compile(`{x: int}`)
		require.NoError(t, err)
		assert.NoError(t, sInt.CompileMap(map[string]any{"x": 2}).Validate())
		// A whole-number float64 is a float — it conflicts with int.
		assert.Error(t, sInt.CompileMap(map[string]any{"x": float64(2)}).Validate())
	})
	t.Run("CompileJSON agrees", func(t *testing.T) {
		// CompileJSON preserves the literal's kind: 2.0 is a float, 2 is an int,
		// so the JSON and map lift paths agree.
		sFloat, err := Compile(`{x: float}`)
		require.NoError(t, err)
		d, err := CompileJSON([]byte(`{"x": 2.0}`))
		require.NoError(t, err)
		assert.NoError(t, sFloat.Unify(d).Validate())
		sInt, err := Compile(`{x: int}`)
		require.NoError(t, err)
		d2, err := CompileJSON([]byte(`{"x": 2.0}`))
		require.NoError(t, err)
		assert.Error(t, sInt.Unify(d2).Validate())
	})
}

// TestNestedThunkPositions pins that a deferred thunk nested in a list element
// or a disjunction branch is forced against its sibling scope — the real fix,
// not a false-reject leaking an *ast node. (Item 6; also unblocks plan 239's
// surface C.)
func TestNestedThunkPositions(t *testing.T) {
	runAcceptCases(t, []acceptCase{
		{"list-element thunk conforms", `{mech: string, xs: [mech != ""]}`, `{"mech": "p", "xs": [true]}`, true},
		{"list-element thunk mech empty", `{mech: string, xs: [mech != ""]}`, `{"mech": "", "xs": [false]}`, true},
		{"list-element thunk rejects wrong", `{mech: string, xs: [mech != ""]}`, `{"mech": "p", "xs": [false]}`, false},
		{"disjunction-branch thunk holds", `{m: string, x: (m == "a") | "z"}`, `{"m": "a", "x": true}`, true},
		{"disjunction-branch thunk falls through", `{m: string, x: (m == "a") | "z"}`, `{"m": "b", "x": "z"}`, true},
	})
}

// TestNestedThunkUndeclaredReference pins that an undeclared reference inside a
// nested thunk position is still a compile-time "reference not found", matching
// CUE's eager rejection, rather than deferring to a validate-time ⊥.
func TestNestedThunkUndeclaredReference(t *testing.T) {
	_, err := Compile(`{xs: [undeclared != ""]}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
