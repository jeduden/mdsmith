package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acceptsMap is a helper: compile schema, validate the map, and report
// whether it passed (nil error).
func acceptsMap(t *testing.T, schema string, m map[string]any) bool {
	t.Helper()
	v, err := Compile(schema)
	require.NoError(t, err)
	return v.CompileMap(m).Validate() == nil
}

// TestCompile_subset exercises every construct the AST compiler supports,
// one accept and one reject per shape, so each compiler and unify rule is
// driven red/green through the public surface.
type subsetCase struct {
	name   string
	schema string
	data   map[string]any
	accept bool
}

func TestCompile_subset(t *testing.T) {
	for _, tc := range subsetCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.accept, acceptsMap(t, tc.schema, tc.data))
		})
	}
}

func subsetCases() []subsetCase {
	return []subsetCase{
		{"string atom ok", `{a: string}`, map[string]any{"a": "x"}, true},
		{"string atom reject", `{a: string}`, map[string]any{"a": 1}, false},
		{"int atom ok", `{a: int}`, map[string]any{"a": int64(3)}, true},
		{"int atom reject float", `{a: int}`, map[string]any{"a": 1.5}, false},
		{"float atom ok", `{a: float}`, map[string]any{"a": 1.5}, true},
		{"number atom ok int", `{a: number}`, map[string]any{"a": int64(2)}, true},
		{"number atom ok float", `{a: number}`, map[string]any{"a": 2.5}, true},
		{"bool atom ok", `{a: bool}`, map[string]any{"a": true}, true},
		{"bytes atom reject string-data", `{a: bytes}`, map[string]any{"a": "x"}, false},
		{"top accepts anything", `{a: _}`, map[string]any{"a": "x"}, true},
		{"null ok", `{a: null}`, map[string]any{"a": nil}, true},
		{"null reject", `{a: null}`, map[string]any{"a": "x"}, false},
		{"string literal ok", `{a: "x"}`, map[string]any{"a": "x"}, true},
		{"string literal reject", `{a: "x"}`, map[string]any{"a": "y"}, false},
		{"int literal ok", `{a: 3}`, map[string]any{"a": int64(3)}, true},
		{"int literal reject", `{a: 3}`, map[string]any{"a": int64(4)}, false},
		{"float literal ok", `{a: 1.5}`, map[string]any{"a": 1.5}, true},
		{"negative int literal ok", `{a: -1}`, map[string]any{"a": int64(-1)}, true},
		{"negative float literal ok", `{a: -1.5}`, map[string]any{"a": -1.5}, true},
		{"underscore int literal ok", `{a: 1_000}`, map[string]any{"a": int64(1000)}, true},
		{"bool literal ok", `{a: true}`, map[string]any{"a": true}, true},
		{"bool literal reject", `{a: false}`, map[string]any{"a": true}, false},
		{"ge bound ok", `{a: >=0}`, map[string]any{"a": int64(0)}, true},
		{"ge bound reject", `{a: >=0}`, map[string]any{"a": int64(-1)}, false},
		{"le bound ok", `{a: <=10}`, map[string]any{"a": int64(10)}, true},
		{"gt bound reject", `{a: >0}`, map[string]any{"a": int64(0)}, false},
		{"lt bound ok", `{a: <10}`, map[string]any{"a": int64(9)}, true},
		{"ne string bound ok", `{a: !=""}`, map[string]any{"a": "x"}, true},
		{"ne string bound reject", `{a: !=""}`, map[string]any{"a": ""}, false},
		{"two bounds ok", `{a: >=0 & <=10}`, map[string]any{"a": int64(5)}, true},
		{"two bounds reject", `{a: >=0 & <=10}`, map[string]any{"a": int64(20)}, false},
		{"regex match ok", `{a: =~"^[a-z]+$"}`, map[string]any{"a": "abc"}, true},
		{"regex match reject", `{a: =~"^[a-z]+$"}`, map[string]any{"a": "AB"}, false},
		{"regex non-match ok", `{a: !~"^[0-9]+$"}`, map[string]any{"a": "abc"}, true},
		{"regex non-match reject", `{a: !~"^[0-9]+$"}`, map[string]any{"a": "123"}, false},
		{"MinRunes ok", `{a: strings.MinRunes(3)}`, map[string]any{"a": "abcd"}, true},
		{"MinRunes reject", `{a: strings.MinRunes(3)}`, map[string]any{"a": "ab"}, false},
		{"disjunction ok", `{a: "x" | "y"}`, map[string]any{"a": "y"}, true},
		{"disjunction reject", `{a: "x" | "y"}`, map[string]any{"a": "z"}, false},
		{"default disjunction absent ok", `{a: bool | *false}`, map[string]any{}, true},
		{"default disjunction present ok", `{a: bool | *false}`, map[string]any{"a": true}, true},
		{"string&bound ok", `{a: string & !=""}`, map[string]any{"a": "x"}, true},
		{"open struct extra key ok", `{a: string}`, map[string]any{"a": "x", "b": 1}, true},
		{"closed struct extra key reject", `close({a: string})`, map[string]any{"a": "x", "b": 1}, false},
		{"nested struct ok", `{a: {b: string}}`, map[string]any{"a": map[string]any{"b": "x"}}, true},
		{"nested struct reject", `{a: {b: int}}`, map[string]any{"a": map[string]any{"b": "x"}}, false},
		{"optional absent ok", `{a?: string}`, map[string]any{}, true},
		{"optional present reject", `{a?: int}`, map[string]any{"a": "x"}, false},
		{"open list ok", `{a: [...int]}`, map[string]any{"a": []any{int64(1), int64(2)}}, true},
		{"open list reject elem", `{a: [...int]}`, map[string]any{"a": []any{"x"}}, false},
		{"open list absent ok", `{a: [...int]}`, map[string]any{}, true},
		{"prefix list ok", `{a: [int, ...int]}`, map[string]any{"a": []any{int64(1), int64(2)}}, true},
		{"closed list length reject", `{a: [int, int]}`, map[string]any{"a": []any{int64(1)}}, false},
		{"quoted label ok", `{"a-b": string}`, map[string]any{"a-b": "x"}, true},
	}
}

// TestCompile_errors covers every clear compile error the subset reports
// for an unsupported or contradictory construct.
func TestCompile_errors(t *testing.T) {
	cases := []struct{ name, schema string }{
		{"contradiction int&string", `{x: int & string}`},
		{"bound conflict", `{x: >=10 & <=0 & 5}`},
		{"unknown reference", `{x: undefinedRef}`},
		{"unsupported function", `{x: nope(1)}`},
		{"unsupported selector function", `{x: strings.Nope(1)}`},
		{"close of non-struct", `{x: close(1)}`},
		{"MinRunes non-int", `{x: strings.MinRunes("a")}`},
		{"standalone star default", `{x: *int}`},
		{"unary plus on string", `{x: +"a"}`},
		{"definition label", `{#x: int}`},
		{"hidden label", `{_x: int}`},
		{"comparison of types", `{x: bool < false}`},
		{"top-level free reference", `0 > A`},
		{"top-level reference in a disjunction branch", `0x0 | 0 < A`},
		{"top-level reference in a list", `[0 < A]`},
		{"embedded free reference", `{nature == "x"}`},
		{"thunk field undeclared ref", `{x: [if y == "z" {string}][0]}`},
		{"regex bad pattern", `{x: =~"["}`},
		{"invalid syntax", `{x: =}`},
		{"index non-list", `{x: 1[0]}`},
		{"bytes literal", `{x: 'b'}`},
		{"top-level bytes literal", `''`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compile(tc.schema)
			assert.Error(t, err, "expected %q to fail compile", tc.schema)
		})
	}
}

// TestCompile_topLevelFields compiles a bare top-level field block (no
// braces), the form ValidateFrontmatter and the package Example use.
func TestCompile_topLevelFields(t *testing.T) {
	v, err := Compile("title: string\nstatus: \"draft\" | \"final\"")
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"title": "T", "status": "final"}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"title": "T", "status": "x"}).Validate())
}

// TestCompile_repeatedFieldMerges pins that a key declared twice composes
// its constraints via &.
func TestCompile_repeatedFieldMerges(t *testing.T) {
	v, err := Compile("a: string\na: !=\"\"")
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": "x"}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"a": ""}).Validate())
}

// TestCompile_ternaryThunk exercises the deferred cross-field ternary: the
// registry field resolves to a required non-empty string only when
// mechanism is "push".
func TestCompile_ternaryThunk(t *testing.T) {
	schema := `close({mechanism: "push" | "pull", ` +
		`registry: [if mechanism == "push" {string & != ""}, (string | *"")][0]})`
	v, err := Compile(schema)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"mechanism": "push", "registry": "npm"}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"mechanism": "push", "registry": ""}).Validate())
	assert.NoError(t, v.CompileMap(map[string]any{"mechanism": "pull", "registry": ""}).Validate())
}

// TestDescribe pins the describe() rendering of each value shape used in
// conflict and incompleteness messages.
func TestDescribe(t *testing.T) {
	cases := []struct {
		schema string
		want   string
	}{
		{`{a: string}`, "string"},
		{`{a: int}`, "int"},
		{`{a: number}`, "number"},
		{`{a: bool}`, "bool"},
		{`{a: bytes}`, "bytes"},
		{`{a: >=0 & <=10}`, "number & >=0 & <=10"},
		{`{a: !=""}`, `string & !=""`},
		{`{a: =~"x"}`, `string & =~"x"`},
		{`{a: !~"x"}`, `string & !~"x"`},
		{`{a: strings.MinRunes(2)}`, "string & strings.MinRunes(2)"},
		{`{a: "x" | "y"}`, `"x" | "y"`},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			v, err := Compile(tc.schema)
			require.NoError(t, err)
			leaf, ok := v.LookupPath(MakePath("a"))
			require.True(t, ok)
			assert.Equal(t, tc.want, leaf.v.describe())
		})
	}
}

// TestDescribe_scalars pins the describe() of concrete scalar leaves and
// the special tokens, reached by validating a conflicting concrete value.
func TestDescribe_scalars(t *testing.T) {
	v := &engineValue{kind: kBottom}
	assert.Equal(t, "_|_", v.describe())
	assert.Equal(t, "_", topValue().describe())
	assert.Equal(t, "null", (&engineValue{kind: kNull}).describe())
	assert.Equal(t, `"s"`, (&engineValue{kind: kString, str: "s"}).describe())
	assert.Equal(t, "3", (&engineValue{kind: kInt, i: 3}).describe())
	assert.Equal(t, "1.5", (&engineValue{kind: kFloat, f: 1.5}).describe())
	assert.Equal(t, "true", (&engineValue{kind: kBool, b: true}).describe())
	assert.Equal(t, "'ab'", (&engineValue{kind: kBytes, bytes: []byte("ab")}).describe())
	assert.Equal(t, "{...}", (&engineValue{kind: kStruct}).describe())
	assert.Equal(t, "[...]", (&engineValue{kind: kList}).describe())
	assert.Equal(t, "(unresolved expression)", (&engineValue{kind: kThunk}).describe())
	// An out-of-range kind hits the describe() default fallback.
	assert.Equal(t, "?", (&engineValue{kind: kind(99)}).describe())
}

// TestKindAndOpStrings pins the String() methods used in messages, including
// the default fallbacks for out-of-range enum values.
func TestKindAndOpStrings(t *testing.T) {
	assert.Equal(t, "string", akString.String())
	assert.Equal(t, "int", akInt.String())
	assert.Equal(t, "float", akFloat.String())
	assert.Equal(t, "number", akNumber.String())
	assert.Equal(t, "bool", akBool.String())
	assert.Equal(t, "bytes", akBytes.String())
	assert.Equal(t, "atomKind(99)", atomKind(99).String())

	assert.Equal(t, ">=", opGe.String())
	assert.Equal(t, "<=", opLe.String())
	assert.Equal(t, ">", opGt.String())
	assert.Equal(t, "<", opLt.String())
	assert.Equal(t, "!=", opNe.String())
	assert.Equal(t, "=~", opMatch.String())
	assert.Equal(t, "!~", opNotMatch.String())
	assert.Equal(t, "strings.MinRunes", opMinRunes.String())
	assert.Equal(t, "boundOp(99)", boundOp(99).String())
}

// TestDecode covers the decode targets and the incomplete-value refusal.
func TestDecode(t *testing.T) {
	t.Run("decode into any", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"a": [1, "x", true, null]}`))
		require.NoError(t, err)
		var out any
		require.NoError(t, v.Decode(&out))
		m := out.(map[string]any)
		assert.Len(t, m["a"], 4)
	})
	t.Run("decode into string", func(t *testing.T) {
		v, err := CompileJSON([]byte(`"hello"`))
		require.NoError(t, err)
		var s string
		require.NoError(t, v.Decode(&s))
		assert.Equal(t, "hello", s)
	})
	t.Run("decode string into wrong target errors", func(t *testing.T) {
		v, err := CompileJSON([]byte(`42`))
		require.NoError(t, err)
		var s string
		assert.Error(t, v.Decode(&s))
	})
	t.Run("decode map into wrong target errors", func(t *testing.T) {
		v, err := CompileJSON([]byte(`"x"`))
		require.NoError(t, err)
		var m map[string]any
		assert.Error(t, v.Decode(&m))
	})
	t.Run("unsupported target type errors", func(t *testing.T) {
		v, err := CompileJSON([]byte(`1`))
		require.NoError(t, err)
		var n int
		assert.Error(t, v.Decode(&n))
	})
	t.Run("incomplete value refuses decode", func(t *testing.T) {
		v, err := Compile(`{a: string}`)
		require.NoError(t, err)
		var out any
		assert.Error(t, v.Decode(&out))
	})
	t.Run("decode bytes and bool and null and int and float", func(t *testing.T) {
		v, err := CompileJSON([]byte(`{"b": true, "n": null, "i": 1, "f": 1.5}`))
		require.NoError(t, err)
		var out any
		require.NoError(t, v.Decode(&out))
		m := out.(map[string]any)
		assert.Equal(t, true, m["b"])
		assert.Nil(t, m["n"])
	})
}

// TestLookupPath_intoList covers a lookup whose segment does not match a
// struct field (a list, a scalar) — none resolve, since the subset looks up
// struct keys only.
func TestLookupPath_intoList(t *testing.T) {
	v, err := CompileJSON([]byte(`{"a": [1, 2]}`))
	require.NoError(t, err)
	_, ok := v.LookupPath(MakePath("a", "0"))
	assert.False(t, ok, "a list element is not a string-labelled member")
}

// TestString_nonString covers Value.String on a non-string leaf and a
// bottom.
func TestString_nonString(t *testing.T) {
	v, err := CompileJSON([]byte(`42`))
	require.NoError(t, err)
	_, err = v.String()
	assert.Error(t, err)
	_, err = bottom(errZeroValue).String()
	assert.Error(t, err)
}

// TestLiftMapValue covers the front-matter value lift across the Go types a
// YAML/JSON decoder produces.
func TestLiftMapValue(t *testing.T) {
	schema, err := Compile(`{i: int, i2: int, f: float, fl: float, b: bool, ` +
		`s: string, n: null, l: [...int], m: {x: int}}`)
	require.NoError(t, err)
	data := map[string]any{
		"i":  42,
		"i2": int64(7),
		"f":  1.5,
		"fl": float32(2.5),
		"b":  true,
		"s":  "x",
		"n":  nil,
		"l":  []any{1, 2},
		"m":  map[string]any{"x": 3},
	}
	assert.NoError(t, schema.CompileMap(data).Validate())
}

// TestLiftMapValue_unsupported covers an unrepresentable nested value.
func TestLiftMapValue_unsupported(t *testing.T) {
	schema, err := Compile(`{a: _}`)
	require.NoError(t, err)
	assert.Error(t, schema.CompileMap(map[string]any{"a": []any{make(chan int)}}).Validate())
	assert.Error(t, schema.CompileMap(map[string]any{"a": map[string]any{"b": make(chan int)}}).Validate())
}

// TestErr_reducedBottom covers Err reporting a conflict reduced inside a
// struct field and inside a list element.
func TestErr_reducedBottom(t *testing.T) {
	t.Run("struct field conflict", func(t *testing.T) {
		s, err := Compile(`{a: "x"}`)
		require.NoError(t, err)
		d, err := CompileJSON([]byte(`{"a": "y"}`))
		require.NoError(t, err)
		assert.Error(t, s.Unify(d).Err())
	})
	t.Run("list element conflict", func(t *testing.T) {
		s, err := Compile(`{a: [int]}`)
		require.NoError(t, err)
		d, err := CompileJSON([]byte(`{"a": ["x"]}`))
		require.NoError(t, err)
		assert.Error(t, s.Unify(d).Err())
	})
	t.Run("no conflict has no err", func(t *testing.T) {
		s, err := Compile(`{a: string}`)
		require.NoError(t, err)
		assert.NoError(t, s.Err())
	})
}
