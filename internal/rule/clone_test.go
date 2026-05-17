package rule

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configurableStub implements both Rule and Configurable.
type configurableStub struct {
	id    string
	name  string
	Max   int
	Style string
}

func (r *configurableStub) ID() string                           { return r.id }
func (r *configurableStub) Name() string                         { return r.name }
func (r *configurableStub) Category() string                     { return "test" }
func (r *configurableStub) Check(_ *lint.File) []lint.Diagnostic { return nil }

func (r *configurableStub) ApplySettings(settings map[string]any) error {
	if v, ok := settings["max"]; ok {
		r.Max = v.(int)
	}
	if v, ok := settings["style"]; ok {
		r.Style = v.(string)
	}
	return nil
}

func (r *configurableStub) DefaultSettings() map[string]any {
	return map[string]any{
		"max":   80,
		"style": "default",
	}
}

var _ Configurable = (*configurableStub)(nil)

func TestCloneRule_Configurable_IndependentCopy(t *testing.T) {
	original := &configurableStub{id: "MDS001", name: "test", Max: 80, Style: "default"}

	clone := CloneRule(original)

	// Clone should be a different pointer.
	assert.NotSame(t, original, clone, "CloneRule should return a new instance, not the same pointer")

	// Clone should have default settings applied.
	cs, ok := clone.(*configurableStub)
	require.True(t, ok, "expected *configurableStub, got %T", clone)
	assert.Equal(t, 80, cs.Max)
	assert.Equal(t, "default", cs.Style)

	// Modify the clone; original should be unaffected.
	cs.Max = 120
	assert.Equal(t, 80, original.Max, "modifying clone affected original")
}

func TestCloneRule_NonConfigurable_IndependentCopy(t *testing.T) {
	original := &stubRule{id: "MDS999", name: "stub"}

	clone := CloneRule(original)

	// Clone should be a different pointer.
	assert.NotSame(t, original, clone, "CloneRule should return a new instance")

	// Should have same ID and Name.
	assert.Equal(t, "MDS999", clone.ID())
	assert.Equal(t, "stub", clone.Name())
}

func TestCloneInstance_Configurable_PreservesIdentityAndState(t *testing.T) {
	original := &configurableStub{id: "MDS001", name: "line-length", Max: 120, Style: "custom"}

	clone := CloneInstance(original)

	assert.NotSame(t, original, clone, "CloneInstance must return a new instance")
	// Unlike CloneRule (zero value + DefaultSettings), CloneInstance is
	// an independent copy of *this* rule: a worker clones before the
	// effective-config name lookup, so losing Name() would make the
	// lookup miss and silently skip the rule.
	assert.Equal(t, "MDS001", clone.ID())
	assert.Equal(t, "line-length", clone.Name())
	cs, ok := clone.(*configurableStub)
	require.True(t, ok, "expected *configurableStub, got %T", clone)
	assert.Equal(t, 120, cs.Max, "CloneInstance must preserve current state, not reset to defaults")
	assert.Equal(t, "custom", cs.Style)

	cs.Max = 999
	assert.Equal(t, 120, original.Max, "mutating clone affected original")
}

func TestCloneInstance_NonConfigurable_IndependentCopy(t *testing.T) {
	original := &stubRule{id: "MDS999", name: "stub"}

	clone := CloneInstance(original)

	assert.NotSame(t, original, clone, "CloneInstance must return a new instance")
	assert.Equal(t, "MDS999", clone.ID())
	assert.Equal(t, "stub", clone.Name())
}

// valueRule satisfies Rule with value receivers, so an interface
// holding it is not a pointer — exercising CloneInstance's value-type
// branch (the interface already carries a copy).
type valueRule struct {
	id   string
	name string
}

func (r valueRule) ID() string                           { return r.id }
func (r valueRule) Name() string                         { return r.name }
func (r valueRule) Category() string                     { return "test" }
func (r valueRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

func TestCloneInstance_ValueType_ReturnsCopy(t *testing.T) {
	var original Rule = valueRule{id: "MDS900", name: "value-rule"}

	clone := CloneInstance(original)

	require.IsType(t, valueRule{}, clone)
	assert.Equal(t, "MDS900", clone.ID())
	assert.Equal(t, "value-rule", clone.Name())
}

func TestCloneRule_ApplySettingsOnClone(t *testing.T) {
	original := &configurableStub{id: "MDS001", name: "test", Max: 80, Style: "default"}

	clone := CloneRule(original)
	cc := clone.(Configurable)
	err := cc.ApplySettings(map[string]any{"max": 120})
	require.NoError(t, err)

	cs := clone.(*configurableStub)
	assert.Equal(t, 120, cs.Max)
	assert.Equal(t, 80, original.Max, "original Max should still be 80")
}
