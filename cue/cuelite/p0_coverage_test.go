package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNestedThunkRefCheck_openListAndDisjunction drives checkThunkRefsIn's
// open-list (elem) and disjunction-branch descents: an undeclared reference in
// each nested position is a compile-time "reference not found".
func TestNestedThunkRefCheck_openListAndDisjunction(t *testing.T) {
	t.Run("open list tail element", func(t *testing.T) {
		_, err := Compile(`{xs: [...undeclared != ""]}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("disjunction branch", func(t *testing.T) {
		_, err := Compile(`{x: (undeclared == "a") | "z"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("declared open-list reference compiles", func(t *testing.T) {
		_, err := Compile(`{mech: string, xs: [...mech != ""]}`)
		require.NoError(t, err)
	})
}

// TestEmptyIntervalStringBounds drives emptyInterval's string branch in both
// the strict-equal and lower-exceeds-upper shapes, complementing the numeric
// cases in TestBoundIntersectionConflict.
func TestEmptyIntervalStringBounds(t *testing.T) {
	t.Run("string lower exceeds upper", func(t *testing.T) {
		_, err := Compile(`{x: >="z" & <="a"}`)
		require.Error(t, err)
	})
	t.Run("string equal and strict is empty", func(t *testing.T) {
		_, err := Compile(`{x: >"m" & <="m"}`)
		require.Error(t, err)
	})
	t.Run("string equal non-strict is the singleton", func(t *testing.T) {
		_, err := Compile(`{x: >="m" & <="m"}`)
		require.NoError(t, err)
	})
}

// TestNestedThunkInOpenListTail forces a thunk that sits in an open list's tail
// element type, exercising forceThunkValue's elem branch and hasThunkValue's
// elem branch.
func TestNestedThunkInOpenListTail(t *testing.T) {
	s, err := Compile(`{mech: string, xs: [...mech != ""]}`)
	require.NoError(t, err)
	t.Run("conforms", func(t *testing.T) {
		assert.NoError(t, s.CompileMap(map[string]any{"mech": "p", "xs": []any{true, true}}).Validate())
	})
	t.Run("rejects a false tail element", func(t *testing.T) {
		assert.Error(t, s.CompileMap(map[string]any{"mech": "p", "xs": []any{false}}).Validate())
	})
}

// TestForceThunkValue_nonThunkPassthrough covers forceThunkValue's default arm
// (a value with no thunk is returned unchanged) and the early returns when a
// list or disjunction carries no thunk: a struct whose fields are plain values
// and whose list/disjunction members hold no thunk forces to itself.
func TestForceThunkValue_nonThunkPassthrough(t *testing.T) {
	scope := map[string]*engineValue{}
	plain := &engineValue{kind: kInt, i: 1}
	assert.Same(t, plain, forceThunkValue(plain, scope, false))
	list := &engineValue{kind: kList, prefix: []*engineValue{plain}}
	assert.Same(t, list, forceThunkValue(list, scope, false))
	disj := &engineValue{
		kind:     kDisjoint,
		branches: []*engineValue{plain, {kind: kInt, i: 2}},
		modes:    []defaultMode{dfltMaybe, dfltMaybe},
	}
	assert.Same(t, disj, forceThunkValue(disj, scope, false))
}

// TestForcedDisjunctionKeepsDefault drives a disjunction whose DEFAULT branch
// is a thunk: forcing preserves the branch's default mode, so the resolved
// default survives the force pass. The schema `x: *(m == "a") | "z"` defaults
// to the comparison; with m="a" the default resolves to true and an absent x
// takes it.
func TestForcedDisjunctionKeepsDefault(t *testing.T) {
	s, err := Compile(`{m: string, x?: *(m == "a") | "z"}`)
	require.NoError(t, err)
	// x is optional and absent, so it takes its default. With m == "a" the
	// default thunk resolves to the concrete true, so the document is concrete.
	assert.NoError(t, s.CompileMap(map[string]any{"m": "a"}).Validate())
}
