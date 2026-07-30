package lsp

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
)

// countingWriter counts how many times Write is called, in addition
// to recording the bytes. Used to distinguish "one Printf call with a
// multi-line body" from "one Printf call per diagnostic" — both leave
// multiple lines in the buffer, but only the former is a single Write.
type countingWriter struct {
	mu    sync.Mutex
	calls int
	buf   strings.Builder
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	return w.buf.Write(p)
}

func (w *countingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *countingWriter) Calls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

// Regression: surfaceForeignDiagnostics must not log or notify once
// per diagnostic. A workspace with several config-target diagnostics
// (e.g. against .mdsmith.yml) re-runs this on every lint pass anywhere
// in the workspace, so a per-diagnostic Printf/writeNotification pair
// turns every keystroke into N log lines and N JSON-RPC round-trips.
// It must instead batch by messageType (Warning vs Error), emitting at
// most one logger.Printf call and one window/logMessage notification
// per group — while still surfacing every diagnostic's uri, file,
// line, message, and rule name somewhere in the output.
func TestSurfaceForeignDiagnosticsBatchesBySeverity(t *testing.T) {
	t.Parallel()
	var out safeBuffer
	logCalls := &countingWriter{}
	s := New(Options{
		Reader: nil,
		Writer: &out,
		Logger: &vlog.Logger{Enabled: true, W: logCalls},
	})

	diags := []lint.Diagnostic{
		{File: "/repo/.mdsmith.yml", Line: 1, Message: "m1", RuleName: "r1", Severity: lint.Warning},
		{File: "/repo/.mdsmith.yml", Line: 2, Message: "m2", RuleName: "r2", Severity: lint.Error},
		{File: "/repo/.mdsmith.yml", Line: 3, Message: "m3", RuleName: "r3", Severity: lint.Warning},
		{File: "/repo/.mdsmith.yml", Line: 4, Message: "m4", RuleName: "r4", Severity: lint.Error},
		{File: "/repo/.mdsmith.yml", Line: 5, Message: "m5", RuleName: "r5", Severity: lint.Warning},
	}

	s.surfaceForeignDiagnostics("file:///doc.md", diags)

	notifCount := strings.Count(out.String(), `"method":"window/logMessage"`)
	assert.LessOrEqual(t, notifCount, 2,
		"must send at most one window/logMessage notification per severity group, not one per diagnostic")
	assert.LessOrEqual(t, logCalls.Calls(), 2,
		"must call logger.Printf at most once per severity group, not one per diagnostic")

	combined := out.String() + logCalls.String()
	for _, want := range []string{
		"m1", "m2", "m3", "m4", "m5",
		"r1", "r2", "r3", "r4", "r5",
		"/repo/.mdsmith.yml",
	} {
		assert.Contains(t, combined, want)
	}
	// Both messageType values must still appear: warnings must not be
	// silently promoted to errors (or vice versa) by the batching.
	assert.Contains(t, out.String(), `"type":1`, "an Error-severity group must be sent")
	assert.Contains(t, out.String(), `"type":2`, "a Warning-severity group must be sent")
}

// Regression: surfaceForeignDiagnostics's per-severity Grow calls
// must reserve the group's *total* size, not just the largest single
// diagnostic. Grow(n) reserves space beyond the builder's *current*
// length, so calling it once per diagnostic on an otherwise-empty
// builder only ever reserves room for one item — summing each
// group's size before a single Grow call is what actually avoids
// the repeated backing-buffer reallocations. 100 same-severity
// diagnostics is large enough that the gap between "sum-then-Grow"
// and "Grow-per-diagnostic" clears measurement noise.
func TestSurfaceForeignDiagnosticsGrowIsPreSized(t *testing.T) {
	diags := make([]lint.Diagnostic, 100)
	for i := range diags {
		diags[i] = lint.Diagnostic{
			File: "/repo/.mdsmith.yml", Line: i + 1, Message: "m", RuleName: "r", Severity: lint.Warning,
		}
	}

	var out safeBuffer
	s := New(Options{Reader: nil, Writer: &out})
	allocs := testing.AllocsPerRun(30, func() {
		s.surfaceForeignDiagnostics("file:///doc.md", diags)
	})
	t.Logf("allocs/op=%.1f", allocs)
	assert.LessOrEqualf(t, allocs, float64(13),
		"surfaceForeignDiagnostics allocated %v times per call for 100 same-severity "+
			"diagnostics; expected the per-group Grow to be sized from the group's "+
			"total, not called once per diagnostic on an empty builder", allocs)
}
