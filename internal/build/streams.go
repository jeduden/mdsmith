package build

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// ringLines is the per-stream in-memory tail size: the last N lines of
// stdout and the last N lines of stderr are retained for the failure and
// timeout diagnostics.
const ringLines = 50

// buildLogsRelDir is the project-root-relative directory holding per-action
// recipe logs.
const buildLogsRelDir = ".mdsmith/build-logs"

// streamCapture tees a recipe's stdout and stderr into three places: an
// in-memory ring buffer (last ringLines lines per stream), a log file with
// each line prefixed [stdout] or [stderr], and — when a live sink is
// supplied — that sink with each line prefixed by the target name.
//
// The log file interleaves both streams in arrival order. A single mutex
// serializes writes so the interleaving and the file stay consistent under
// concurrent stdout/stderr goroutines started by os/exec.
type streamCapture struct {
	mu      sync.Mutex
	logFile *os.File
	sink    io.Writer
	name    string

	out *lineStream
	err *lineStream
}

// newStreamCapture opens logPath for writing (creating parent dirs) and
// returns a capture whose stdout()/stderr() writers feed the ring buffers,
// the log file, and, if sink is non-nil, the live forward sink.
func newStreamCapture(logPath, name string, sink io.Writer) (*streamCapture, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating build-logs dir: %w", err)
	}
	f, err := os.Create(logPath) //nolint:gosec // logPath is the fixed in-root build-logs file
	if err != nil {
		return nil, fmt.Errorf("creating build log: %w", err)
	}
	sc := &streamCapture{logFile: f, sink: sink, name: name}
	sc.out = &lineStream{cap: sc, tag: "stdout"}
	sc.err = &lineStream{cap: sc, tag: "stderr"}
	return sc, nil
}

// stdout returns the writer for the recipe's standard output.
func (s *streamCapture) stdout() io.Writer { return s.out }

// stderr returns the writer for the recipe's standard error.
func (s *streamCapture) stderr() io.Writer { return s.err }

// stdoutTail returns a copy of the buffered stdout tail (up to ringLines).
func (s *streamCapture) stdoutTail() []string { return s.out.snapshot() }

// stderrTail returns a copy of the buffered stderr tail (up to ringLines).
func (s *streamCapture) stderrTail() []string { return s.err.snapshot() }

// Close flushes any partial trailing line from each stream and closes the
// log file.
func (s *streamCapture) Close() error {
	s.out.flushPartial()
	s.err.flushPartial()
	return s.logFile.Close()
}

// emit writes one complete line (no trailing newline) tagged by stream to
// the log file and, when present, the live sink.
func (s *streamCapture) emit(tag, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintf(s.logFile, "[%s] %s\n", tag, line)
	if s.sink != nil {
		_, _ = fmt.Fprintf(s.sink, "[%s] %s\n", s.name, line)
	}
}

// lineStream is the io.Writer side of one stream. It splits incoming bytes
// on '\n', appends complete lines to a ring buffer, and forwards each
// complete line to the capture's emit. A partial trailing line is held in
// pending until the next write completes it or Close flushes it.
type lineStream struct {
	cap     *streamCapture
	tag     string
	mu      sync.Mutex
	ring    []string
	pending []byte
}

// Write implements io.Writer. Splitting on '\n' means a recipe that writes
// a line in several Write calls still produces one ring/log line.
func (l *lineStream) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, p...)
	for {
		i := bytes.IndexByte(l.pending, '\n')
		if i < 0 {
			break
		}
		line := string(l.pending[:i])
		l.pending = l.pending[i+1:]
		l.append(line)
		l.cap.emit(l.tag, line)
	}
	return len(p), nil
}

// append adds line to the ring buffer, dropping the oldest line past the cap.
func (l *lineStream) append(line string) {
	if len(l.ring) < ringLines {
		l.ring = append(l.ring, line)
		return
	}
	copy(l.ring, l.ring[1:])
	l.ring[len(l.ring)-1] = line
}

// flushPartial emits any held partial line (a recipe that exits without a
// trailing newline).
func (l *lineStream) flushPartial() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.pending) == 0 {
		return
	}
	line := string(l.pending)
	l.pending = nil
	l.append(line)
	l.cap.emit(l.tag, line)
}

// snapshot returns a copy of the current ring buffer.
func (l *lineStream) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.ring))
	copy(out, l.ring)
	return out
}

// logFileName returns the log file's base name for a given ActionID. The
// ActionID is "sha256-<hex>"; that is already a safe file name.
func logFileName(actionID string) string {
	return actionID + ".log"
}

// logPathFor returns the absolute log path under root for an ActionID.
func logPathFor(root, actionID string) string {
	return filepath.Join(root, filepath.FromSlash(buildLogsRelDir), logFileName(actionID))
}
