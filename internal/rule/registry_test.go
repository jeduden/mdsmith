package rule

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRule is a minimal Rule implementation for testing.
type stubRule struct {
	id   string
	name string
}

func (r *stubRule) ID() string                           { return r.id }
func (r *stubRule) Name() string                         { return r.name }
func (r *stubRule) Category() string                     { return "test" }
func (r *stubRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

func resetRegistry() {
	registry = nil
}

func TestRegisterAndAll(t *testing.T) {
	resetRegistry()

	r1 := &stubRule{id: "MDS001", name: "line-length"}
	r2 := &stubRule{id: "MDS002", name: "first-heading"}

	Register(r1)
	Register(r2)

	all := All()
	require.Len(t, all, 2)
	assert.Equal(t, "MDS001", all[0].ID())
	assert.Equal(t, "MDS002", all[1].ID())
}

func TestAllReturnsCopy(t *testing.T) {
	resetRegistry()

	Register(&stubRule{id: "MDS001", name: "line-length"})

	all := All()
	all[0] = nil // Mutate the returned slice.

	// The registry should be unaffected.
	original := All()
	assert.NotNil(t, original[0], "All() should return a copy; mutating the result affected the registry")
}

func TestByID_Found(t *testing.T) {
	resetRegistry()

	r := &stubRule{id: "MDS003", name: "heading-style"}
	Register(r)

	found := ByID("MDS003")
	require.NotNil(t, found, "expected to find rule MDS003")
	assert.Equal(t, "MDS003", found.ID())
	assert.Equal(t, "heading-style", found.Name())
}

func TestByID_NotFound(t *testing.T) {
	resetRegistry()

	Register(&stubRule{id: "MDS001", name: "line-length"})

	found := ByID("MDS999")
	assert.Nil(t, found, "expected nil for unknown rule ID")
}
