package rule

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
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
	original := &configurableStub{id: "TM001", name: "test", Max: 80, Style: "default"}

	clone := CloneRule(original)

	// Clone should be a different pointer.
	if clone == original {
		t.Error("CloneRule should return a new instance, not the same pointer")
	}

	// Clone should have default settings applied.
	cs, ok := clone.(*configurableStub)
	if !ok {
		t.Fatalf("expected *configurableStub, got %T", clone)
	}
	if cs.Max != 80 {
		t.Errorf("expected Max=80, got %d", cs.Max)
	}
	if cs.Style != "default" {
		t.Errorf("expected Style=default, got %s", cs.Style)
	}

	// Modify the clone; original should be unaffected.
	cs.Max = 120
	if original.Max != 80 {
		t.Errorf("modifying clone affected original: Max=%d", original.Max)
	}
}

func TestCloneRule_NonConfigurable_IndependentCopy(t *testing.T) {
	original := &stubRule{id: "TM999", name: "stub"}

	clone := CloneRule(original)

	// Clone should be a different pointer.
	if clone == original {
		t.Error("CloneRule should return a new instance")
	}

	// Should have same ID and Name.
	if clone.ID() != "TM999" {
		t.Errorf("expected ID TM999, got %s", clone.ID())
	}
	if clone.Name() != "stub" {
		t.Errorf("expected Name stub, got %s", clone.Name())
	}
}

func TestCloneRule_ApplySettingsOnClone(t *testing.T) {
	original := &configurableStub{id: "TM001", name: "test", Max: 80, Style: "default"}

	clone := CloneRule(original)
	cc := clone.(Configurable)
	if err := cc.ApplySettings(map[string]any{"max": 120}); err != nil {
		t.Fatalf("ApplySettings on clone: %v", err)
	}

	cs := clone.(*configurableStub)
	if cs.Max != 120 {
		t.Errorf("expected clone Max=120, got %d", cs.Max)
	}
	if original.Max != 80 {
		t.Errorf("original Max should still be 80, got %d", original.Max)
	}
}
