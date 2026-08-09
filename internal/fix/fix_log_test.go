package fix

import (
	"testing"

	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixer_log_NoLoggerSet_ReturnsSharedDisabledLogger pins that when
// no Logger is configured, log() returns a shared disabled logger
// instead of allocating a fresh *vlog.Logger on every call — the
// fixOnce loop calls log() once per file, so a fresh allocation there
// is a per-file-in-the-workspace allocation for a value nothing ever
// mutates. Mirrors internal/engine's identical Runner.log() fix (see
// docs/development/high-performance-go.md, "the cheapest call is the
// one you never make").
func TestFixer_log_NoLoggerSet_ReturnsSharedDisabledLogger(t *testing.T) {
	f := &Fixer{}

	first := f.log()
	second := f.log()

	require.NotNil(t, first)
	assert.Same(t, first, second,
		"repeated calls must return the same disabled-logger instance, not a fresh allocation each time")
	assert.False(t, first.Enabled, "the shared fallback logger must stay disabled")
}

// TestFixer_log_LoggerSet_ReturnsIt pins the existing behavior: when a
// Logger is configured, log() returns it unchanged.
func TestFixer_log_LoggerSet_ReturnsIt(t *testing.T) {
	set := &vlog.Logger{}
	f := &Fixer{Logger: set}

	assert.Same(t, set, f.log())
}

// TestFixer_log_NoLoggerSet_ZeroAllocs is a deterministic (non-timing)
// companion: it pins that log() with no Logger configured allocates
// nothing. Confirmed red (1 alloc/op) against the pre-fix
// &vlog.Logger{} literal, green (0 allocs/op) after.
func TestFixer_log_NoLoggerSet_ZeroAllocs(t *testing.T) {
	f := &Fixer{}
	allocs := testing.AllocsPerRun(100, func() {
		// Exercise it the way fixOnce does: read Enabled, then call
		// Printf (a real, non-inlined method call taking the receiver
		// as an argument) so the compiler cannot prove the returned
		// *Logger is dead and optimize the whole thing away.
		flog := f.log()
		if flog.Enabled {
			t.Fatal("disabled logger must report Enabled == false")
		}
		flog.Printf("file: %s", "bench.md")
	})
	if allocs != 0 {
		t.Fatalf("Fixer.log() allocs/op = %.1f, want 0", allocs)
	}
}
