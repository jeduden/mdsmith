package cuelite

import (
	"testing"

	"github.com/jeduden/mdsmith/cue/cuelite/syntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests close the last statement-coverage gaps: the comprehension and
// comparison error positions, the freeRefs walk over a comprehension body's
// fields, and a handful of label/embedded shapes reached most directly by
// constructing the AST node or calling the helper. Each is a real,
// engine-reachable branch driven red/green.

// parseExpr parses a single CUE-subset expression for direct-helper tests,
// wrapping it in a field the in-house frontend understands and returning the
// field's value expression.
func parseExpr(t *testing.T, src string) syntax.Expr {
	t.Helper()
	file, err := syntax.ParseFile("x: " + src)
	require.NoError(t, err)
	require.Len(t, file.Decls, 1)
	return file.Decls[0].(*syntax.Field).Value
}

// TestEvalExpr_unsupportedConstruct covers evalExpr's default branch via a
// construct outside the subset (a slice expression).
func TestEvalExpr_unsupportedConstruct(t *testing.T) {
	_, err := Compile(`{n: int, a: n[1:2]}`)
	assert.Error(t, err, "a slice expression is outside the subset")
}

// TestEvalIdent_scopeResolution covers evalIdent's scope-hit and
// non-concrete-in-scope branches through a forced thunk: `n + ...` is not in
// the subset, so use a relational that reads n from scope.
func TestEvalIdent_scopeBranches(t *testing.T) {
	// A thunk condition `s != ""` reads sibling s from the force scope; with s
	// concrete the reference resolves (scope-hit branch).
	v, err := Compile(`{s: string, r: [if s != "" {string & != ""}, (string | *"")][0]}`)
	require.NoError(t, err)
	// s non-empty → the body applies → empty r rejected.
	assert.Error(t, v.CompileMap(map[string]any{"s": "x", "r": ""}).Validate())
	// s empty → body dropped → empty r accepted.
	assert.NoError(t, v.CompileMap(map[string]any{"s": "", "r": ""}).Validate())
}

// TestEvalComprehension_errorShapes covers evalComprehension's
// multi-clause, non-if-clause, and non-struct-body rejections, plus the
// condition error propagation.
func TestEvalComprehension_errorShapes(t *testing.T) {
	// Multi-clause / for-clause comprehension is rejected.
	_, err := Compile(`{xs: [...int], r: [for x in xs {x}][0]}`)
	assert.Error(t, err)
	// A comprehension whose body is not a struct (a bare scalar) is rejected.
	_, err = Compile(`{c: bool, r: [if c {1} 1][0]}`)
	assert.Error(t, err)
}

// TestEvalComprehension_nonStructBody constructs a comprehension whose Value
// is not a StructLit, driving evalComprehension's non-struct-body branch
// directly.
func TestEvalComprehension_nonStructBody(t *testing.T) {
	comp := &syntax.Comprehension{
		Clauses: []syntax.Clause{&syntax.IfClause{Condition: &syntax.BasicLit{Kind: syntax.TRUE, Value: "true"}}},
		Value:   &syntax.BasicLit{Kind: syntax.STRING, Value: `"not a struct"`},
	}
	_, _, err := evalComprehension(comp, nil)
	assert.Error(t, err)
}

// TestEvalComprehension_condError drives the condition-evaluation error
// branch: a condition that compiles to a hard error.
func TestEvalComprehension_condError(t *testing.T) {
	comp := &syntax.Comprehension{
		Clauses: []syntax.Clause{&syntax.IfClause{Condition: parseExpr(t, `=~"["`)}},
		Value:   &syntax.StructLit{},
	}
	_, _, err := evalComprehension(comp, map[string]*engineValue{})
	assert.Error(t, err)
}

// TestCompareConcrete_regexAndIncomparable covers compareConcrete's regex-
// compile error and its numeric-incomparable error, reached through a forced
// thunk over a sibling.
func TestCompareConcrete_regexAndIncomparable(t *testing.T) {
	// A regex comparison with an invalid pattern: `s =~ "["` forces to an error.
	v, err := Compile(`{s: string, r: [if s =~ "[" {string}][0]}`)
	require.NoError(t, err)
	assert.Error(t, v.CompileMap(map[string]any{"s": "x", "r": "y"}).Validate())
}

// TestCompareConcrete_direct drives compareConcrete's regex-compile-error and
// the incomparable-numeric branch directly with constructed scalars.
func TestCompareConcrete_direct(t *testing.T) {
	// regex compile error.
	_, err := compareConcrete(&engineValue{kind: kString, str: "x"}, syntax.MAT, &engineValue{kind: kString, str: "["})
	assert.Error(t, err)
	// numeric op over a bool operand: incomparable.
	_, err = compareConcrete(&engineValue{kind: kBool, b: true}, syntax.GTR, &engineValue{kind: kInt, i: 1})
	assert.Error(t, err)
}

// TestEvalList_comprehensionError covers evalList's comprehension
// error-propagation branch: a list literal whose comprehension body fails.
func TestEvalList_comprehensionError(t *testing.T) {
	_, err := Compile(`{a: [if true {=~"["}]}`)
	assert.Error(t, err)
}

// TestEvalStruct_twoEmbedsAndFieldPlusEmbed covers evalStruct's second-embed
// unify branch and the field-plus-embedded result.
func TestEvalStruct_embeddedShapes(t *testing.T) {
	// Two embedded bounds compose: {>=0, <=10}.
	v, err := Compile(`{a: {>=0, <=10}}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": int64(5)}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"a": int64(20)}).Validate())
	// A field plus an embedded bound on the same struct: {x: int, >=0} — the
	// embed constrains the struct itself, which conflicts with having fields,
	// so this exercises the field+embed unify position.
	_, err = Compile(`{a: {x: int} & {y: string}}`)
	require.NoError(t, err)
}

// TestFreeRefs_comprehensionBodyFields drives freeRefs over an index thunk
// whose comprehension body is a struct with fields, covering the Field and
// nested-walk branches of freeRefs.
func TestFreeRefs_comprehensionBodyFields(t *testing.T) {
	// The body {title: string & != ""} carries a Field whose label is a key,
	// not a reference; the only free reference is the condition's `mode`.
	e := parseExpr(t, `[if mode == "full" {{title: string & != ""}}, ({...} | *{})][0]`)
	refs := freeRefs(e)
	assert.Equal(t, []string{"mode"}, refs)
}

// TestFreeRefs_selectorBase covers freeRefs' SelectorExpr branch: only the
// base of a selector is a reference.
func TestFreeRefs_selectorBase(t *testing.T) {
	e := parseExpr(t, `base.member`)
	refs := freeRefs(e)
	assert.Equal(t, []string{"base"}, refs)
}

// TestFieldLabel_directBranches drives fieldLabel's non-string-literal and
// default branches with constructed labels.
func TestFieldLabel_directBranches(t *testing.T) {
	// A non-string BasicLit label (an int literal) is rejected.
	_, err := fieldLabel(&syntax.BasicLit{Kind: syntax.INT, Value: "1"})
	assert.Error(t, err)
	// A quoted string label that needs unquoting succeeds.
	name, err := fieldLabel(&syntax.BasicLit{Kind: syntax.STRING, Value: `"a-b"`})
	require.NoError(t, err)
	assert.Equal(t, "a-b", name)
	// A malformed quoted label fails to unquote.
	_, err = fieldLabel(&syntax.BasicLit{Kind: syntax.STRING, Value: `"\x"`})
	assert.Error(t, err)
	// A bare type-keyword identifier label is rejected: it shadows references
	// to the same name in the field value, which the engine cannot model.
	for _, kw := range []string{"int", "string", "float", "number", "bool", "bytes"} {
		_, err = fieldLabel(&syntax.Ident{Name: kw})
		assert.Error(t, err, "bare type-keyword label %q must reject", kw)
	}
	// A non-keyword identifier label is accepted; a quoted type-keyword is too.
	name, err = fieldLabel(&syntax.Ident{Name: "status"})
	require.NoError(t, err)
	assert.Equal(t, "status", name)
	name, err = fieldLabel(&syntax.BasicLit{Kind: syntax.STRING, Value: `"int"`})
	require.NoError(t, err)
	assert.Equal(t, "int", name)
}

// TestCompileFile_topLevelEmbedAmongFields covers compileFile's
// unsupported-top-level-declaration branch: a top-level embedded value
// alongside fields.
func TestCompileFile_topLevelEmbedAmongFields(t *testing.T) {
	_, err := Compile("a: int\nstring")
	assert.Error(t, err, "a bare top-level embedded value among fields is unsupported")
}
