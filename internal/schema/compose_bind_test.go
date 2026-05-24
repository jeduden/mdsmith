package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bindPtr(s string) *string { return &s }

// litBound builds a literal-heading scope with an explicit `bind:`
// override (the inline form's parsed shape).
func litBound(heading, bind string) Scope {
	sc := lit(heading)
	sc.Bind = bindPtr(bind)
	return sc
}

// TestCompose_BindNilWithBindCarriesOver: when one kind sets a bind
// and the other leaves it unset, the composed scope keeps the
// non-nil value. Covers both orderings — bind on the second input
// and bind on the first input — since mergeBind's nil branches are
// distinct.
func TestCompose_BindNilWithBindCarriesOver(t *testing.T) {
	t.Run("nil_then_bound", func(t *testing.T) {
		a := &Schema{Sections: []Scope{lit("Goal")}}
		b := &Schema{Sections: []Scope{litBound("Goal", "objective")}}
		out, err := Compose(a, b)
		require.NoError(t, err)
		require.NotNil(t, out.Sections[0].Bind)
		assert.Equal(t, "objective", *out.Sections[0].Bind)
	})
	t.Run("bound_then_nil", func(t *testing.T) {
		a := &Schema{Sections: []Scope{litBound("Goal", "objective")}}
		b := &Schema{Sections: []Scope{lit("Goal")}}
		out, err := Compose(a, b)
		require.NoError(t, err)
		require.NotNil(t, out.Sections[0].Bind)
		assert.Equal(t, "objective", *out.Sections[0].Bind)
	})
}

// TestCompose_BindEqualUnifies: two kinds with the same bind for
// the same heading compose without conflict.
func TestCompose_BindEqualUnifies(t *testing.T) {
	a := &Schema{Sections: []Scope{litBound("Goal", "objective")}}
	b := &Schema{Sections: []Scope{litBound("Goal", "objective")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotNil(t, out.Sections[0].Bind)
	assert.Equal(t, "objective", *out.Sections[0].Bind)
}

// TestCompose_BindConflict: a real disagreement between two kinds
// surfaces as a compose-time error rather than silently picking one.
func TestCompose_BindConflict(t *testing.T) {
	a := &Schema{Sections: []Scope{litBound("Goal", "objective")}}
	b := &Schema{Sections: []Scope{litBound("Goal", "purpose")}}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting `bind:`")
	assert.Contains(t, err.Error(), "Goal")
	assert.Contains(t, err.Error(), `"objective"`)
	assert.Contains(t, err.Error(), `"purpose"`)
}

// TestCompose_BindHoistVsKeyConflict: one source asks to hoist
// (`bind: ""`) while another asks for a named projection key —
// the message must distinguish the hoist case so the diagnostic
// is actionable.
func TestCompose_BindHoistVsKeyConflict(t *testing.T) {
	a := &Schema{Sections: []Scope{litBound("Wrapper", "")}}
	b := &Schema{Sections: []Scope{litBound("Wrapper", "result")}}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Wrapper")
	assert.Contains(t, err.Error(), `hoist`)
	assert.Contains(t, err.Error(), `"result"`)
}

// TestCompose_BindEmptyWithUnset: an explicit `bind: ""` (hoist)
// from one kind overrides a nil from another.
func TestCompose_BindEmptyWithUnset(t *testing.T) {
	a := &Schema{Sections: []Scope{lit("Wrapper")}}
	b := &Schema{Sections: []Scope{litBound("Wrapper", "")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotNil(t, out.Sections[0].Bind)
	assert.Equal(t, "", *out.Sections[0].Bind)
}

// TestCompose_DuplicateSiblingBindsAcrossKinds: two kinds binding
// *different* sibling scopes to the same key collide at compose
// time — one heading from each kind cannot share a projection key.
func TestCompose_DuplicateSiblingBindsAcrossKinds(t *testing.T) {
	a := &Schema{Sections: []Scope{litBound("Goal", "result")}}
	b := &Schema{Sections: []Scope{litBound("Risks", "result")}}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two sibling scopes")
	assert.Contains(t, err.Error(), "result")
}

// TestCompose_EmptyBindsDoNotCollide: two siblings both hoisting
// (`bind: ""`) produce no keys and therefore cannot collide on the
// projection. Compose accepts them.
func TestCompose_EmptyBindsDoNotCollide(t *testing.T) {
	a := &Schema{Sections: []Scope{litBound("A", "")}}
	b := &Schema{Sections: []Scope{litBound("B", "")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Len(t, out.Sections, 2)
}
