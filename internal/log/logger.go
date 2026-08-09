package log

import (
	"fmt"
	"io"
	"sync"
)

// Logger writes verbose diagnostic messages when Enabled is true.
// Output goes to the configured writer (typically stderr).
//
// Printf is safe for concurrent use: the parallel lint pipeline can
// reach a shared logger from many goroutines, and not every io.Writer
// (e.g. bytes.Buffer) is itself thread-safe. mu serializes the format
// + write so lines are never torn or dropped. Always use *Logger;
// copying a Logger value would copy the mutex.
type Logger struct {
	Enabled bool
	W       io.Writer
	mu      sync.Mutex
}

// Printf writes a formatted message to W when Enabled is true.
// It is a no-op when Enabled is false.
func (l *Logger) Printf(format string, args ...any) {
	if !l.Enabled {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.W, format+"\n", args...)
}

// Disabled is a shared, always-off Logger for callers that need a
// non-nil fallback when no Logger is configured. Every disabled
// Logger is behaviorally identical (Printf no-ops without touching W
// or mu), so a caller that resolves a fallback on a per-item hot path
// (once per file, once per request) should return this shared
// instance instead of allocating a fresh &Logger{} each time — see
// docs/development/high-performance-go.md, "the cheapest call is the
// one you never make". Never mutate a Logger reachable through this
// var; every caller expects it to stay disabled.
var Disabled = &Logger{}
