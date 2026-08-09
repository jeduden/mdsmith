package engine

import (
	"testing"

	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunner_log_NoLoggerSet_ReturnsSharedDisabledLogger pins that when
// no Logger is configured, log() returns a shared disabled logger
// instead of allocating a fresh *vlog.Logger on every call — lintFile
// calls log() once per file, so a fresh allocation there is a
// per-file-in-the-workspace allocation for a value nothing ever
// mutates (see docs/development/high-performance-go.md, "Reuse
// loop-local buffers" / "the cheapest call is the one you never
// make").
func TestRunner_log_NoLoggerSet_ReturnsSharedDisabledLogger(t *testing.T) {
	r := &Runner{}

	first := r.log()
	second := r.log()

	require.NotNil(t, first)
	assert.Same(t, first, second,
		"repeated calls must return the same disabled-logger instance, not a fresh allocation each time")
	assert.False(t, first.Enabled, "the shared fallback logger must stay disabled")
}

// TestRunner_log_LoggerSet_ReturnsIt pins the existing behavior: when
// a Logger is configured, log() returns it unchanged.
func TestRunner_log_LoggerSet_ReturnsIt(t *testing.T) {
	set := &vlog.Logger{}
	r := &Runner{Logger: set}

	assert.Same(t, set, r.log())
}

// TestRunner_log_NoLoggerSet_ZeroAllocs is a deterministic (non-timing)
// companion to TestRunner_log_NoLoggerSet_ReturnsSharedDisabledLogger:
// it pins that log() with no Logger configured allocates nothing,
// confirming the shared-singleton fix actually removed the per-call
// allocation rather than merely returning an equal-but-fresh value.
// Confirmed red (1 alloc/op) against the pre-fix &vlog.Logger{}
// literal, green (0 allocs/op) after.
func TestRunner_log_NoLoggerSet_ZeroAllocs(t *testing.T) {
	r := &Runner{}
	allocs := testing.AllocsPerRun(100, func() {
		// Exercise it the way lintFile does: read Enabled, then call
		// Printf (a real, non-inlined method call taking the receiver
		// as an argument) so the compiler cannot prove the returned
		// *Logger is dead and optimize the whole thing away.
		flog := r.log()
		if flog.Enabled {
			t.Fatal("disabled logger must report Enabled == false")
		}
		flog.Printf("file: %s", "bench.md")
	})
	if allocs != 0 {
		t.Fatalf("Runner.log() allocs/op = %.1f, want 0", allocs)
	}
}
