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
