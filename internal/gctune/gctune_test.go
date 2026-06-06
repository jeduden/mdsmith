package gctune_test

import (
	"runtime/debug"
	"testing"

	"github.com/jeduden/mdsmith/internal/gctune"
	"github.com/stretchr/testify/assert"
)

func TestTarget(t *testing.T) {
	// GOGC unset (empty): apply the batch default.
	assert.Equal(t, gctune.BatchPercent, gctune.Target(""))

	// GOGC pinned by the user (any value): leave the runtime default.
	assert.Equal(t, -1, gctune.Target("50"))
	assert.Equal(t, -1, gctune.Target("off"))
	assert.Equal(t, -1, gctune.Target("100"))
}

func TestApplyBatch_SetsTargetWhenUnset(t *testing.T) {
	t.Setenv("GOGC", "") // treated as unset
	prev := debug.SetGCPercent(100)
	defer debug.SetGCPercent(prev)
	gctune.ApplyBatch()
	got := debug.SetGCPercent(100) // returns the value ApplyBatch set
	assert.Equal(t, gctune.BatchPercent, got)
}

func TestApplyBatch_RespectsExplicitGOGC(t *testing.T) {
	t.Setenv("GOGC", "50")
	prev := debug.SetGCPercent(123)
	defer debug.SetGCPercent(prev)
	gctune.ApplyBatch() // GOGC pinned ⇒ leaves the target untouched
	got := debug.SetGCPercent(123)
	assert.Equal(t, 123, got)
}
