package cuelite

import (
	"encoding/json"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompileUnary_plusAndUnsupported covers compileUnary's valid-unary-plus
// return and its unsupported-operator default.
func TestCompileUnary_plusAndUnsupported(t *testing.T) {
	v, err := Compile(`{a: +1}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": int64(1)}).Validate())
	// A logical-not unary is outside the subset.
	_, err = Compile(`{a: !true}`)
	assert.Error(t, err)
}

// TestSelectorName covers both branches of selectorName: a plain identifier
// member and the non-identifier fallback.
func TestSelectorName(t *testing.T) {
	assert.Equal(t, "member", selectorName(ast.NewIdent("member")))
	assert.Equal(t, "?", selectorName(&ast.BasicLit{Kind: token.STRING, Value: `"x"`}))
}

// TestLiftNumber_malformed covers liftNumber's non-range parse-error branch via
// a json.Number that is not a valid number (the decoder never produces one,
// but the contract guards against it).
func TestLiftNumber_malformed(t *testing.T) {
	_, err := liftNumber(json.Number("not-a-number"))
	assert.Error(t, err)
}

// TestEvalIdent_scopeHitAndMiss covers evalIdent's concrete-scope-hit and
// absent-from-scope branches directly.
func TestEvalIdent_scopeHitAndMiss(t *testing.T) {
	scope := map[string]*engineValue{"n": {kind: kInt, i: 7}}
	v, err := evalIdent(&ast.Ident{Name: "n"}, scope)
	require.NoError(t, err)
	assert.Equal(t, int64(7), v.i)

	_, err = evalIdent(&ast.Ident{Name: "absent"}, scope)
	assert.ErrorIs(t, err, errUnresolved)
}

// TestEvalIdent_literalKeywords covers evalIdent's null/true/false keyword
// branches, which the parser emits as Idents (not BasicLits) in some
// positions but a literal at a value position emits as a BasicLit.
func TestEvalIdent_literalKeywords(t *testing.T) {
	n, err := evalIdent(ast.NewIdent("null"), nil)
	require.NoError(t, err)
	assert.Equal(t, kNull, n.kind)
	tr, err := evalIdent(ast.NewIdent("true"), nil)
	require.NoError(t, err)
	assert.Equal(t, kBool, tr.kind)
	assert.True(t, tr.b)
	f, err := evalIdent(ast.NewIdent("false"), nil)
	require.NoError(t, err)
	assert.False(t, f.b)
}

// TestEvalComprehension_nonBoolCondition covers evalComprehension's
// non-bool-condition branches: a CONCRETE non-bool condition is a type error
// (CUE rejects "cannot use 1 (type int) as type bool"), while a NON-concrete
// condition (a type/top) defers with errUnresolved.
func TestEvalComprehension_nonBoolCondition(t *testing.T) {
	t.Run("concrete non-bool is a type error", func(t *testing.T) {
		comp := &ast.Comprehension{
			Clauses: []ast.Clause{&ast.IfClause{Condition: ast.NewLit(token.INT, "1")}},
			Value:   &ast.StructLit{},
		}
		_, _, err := evalComprehension(comp, map[string]*engineValue{})
		require.Error(t, err)
		assert.NotErrorIs(t, err, errUnresolved)
		assert.Contains(t, err.Error(), "if condition must be a bool")
	})
	t.Run("non-concrete type condition defers", func(t *testing.T) {
		comp := &ast.Comprehension{
			Clauses: []ast.Clause{&ast.IfClause{Condition: ast.NewIdent("string")}},
			Value:   &ast.StructLit{},
		}
		_, _, err := evalComprehension(comp, map[string]*engineValue{})
		assert.ErrorIs(t, err, errUnresolved, "a non-concrete type condition defers")
	})
}

// TestEvalStruct_ellipsisUnderScope covers evalStruct's Ellipsis branch during
// a force pass (scope != nil).
func TestEvalStruct_ellipsisUnderScope(t *testing.T) {
	st := &ast.StructLit{Elts: []ast.Decl{
		&ast.Field{Label: ast.NewIdent("a"), Value: ast.NewIdent("int")},
		&ast.Ellipsis{},
	}}
	v, err := evalStruct(st, map[string]*engineValue{})
	require.NoError(t, err)
	assert.Equal(t, kStruct, v.kind)
}

// TestEvalStruct_unsupportedElement covers evalStruct's default branch: a
// struct declaration that is neither a field, an embed, nor an ellipsis (an
// attribute).
func TestEvalStruct_unsupportedElement(t *testing.T) {
	st := &ast.StructLit{Elts: []ast.Decl{
		&ast.Attribute{Text: "@go(x)"},
	}}
	_, err := evalStruct(st, nil)
	assert.Error(t, err)
}

// TestEvalComprehension_directErrors covers evalComprehension's multi-clause
// and non-if-clause rejections directly.
func TestEvalComprehension_directErrors(t *testing.T) {
	multi := &ast.Comprehension{Clauses: []ast.Clause{
		&ast.IfClause{Condition: ast.NewBool(true)},
		&ast.IfClause{Condition: ast.NewBool(true)},
	}, Value: &ast.StructLit{}}
	_, _, err := evalComprehension(multi, nil)
	assert.Error(t, err, "a multi-clause comprehension is rejected")

	forClause := &ast.Comprehension{Clauses: []ast.Clause{
		&ast.ForClause{Source: ast.NewList()},
	}, Value: &ast.StructLit{}}
	_, _, err = evalComprehension(forClause, nil)
	assert.Error(t, err, "a non-if clause is rejected")
}

// TestEvalStruct_embeddedSecondMeet covers evalStruct's second-embed unify
// branch (two embedded values in one struct literal).
func TestEvalStruct_embeddedSecondMeet(t *testing.T) {
	// Two embeds in one struct: {>=0, <=10} composes both bounds.
	st := &ast.StructLit{Elts: []ast.Decl{
		&ast.EmbedDecl{Expr: &ast.UnaryExpr{Op: token.GEQ, X: ast.NewLit(token.INT, "0")}},
		&ast.EmbedDecl{Expr: &ast.UnaryExpr{Op: token.LEQ, X: ast.NewLit(token.INT, "10")}},
	}}
	v, err := evalStruct(st, nil)
	require.NoError(t, err)
	assert.Equal(t, kBound, v.kind)
	require.Len(t, v.bounds, 2)
}
