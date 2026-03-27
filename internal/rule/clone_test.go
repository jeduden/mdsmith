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
