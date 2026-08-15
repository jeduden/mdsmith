package engine

import (
	"testing"

	vlog "github.com/jeduden/mdsmith/internal/log"
)

// TestRunner_log_NoLoggerAllocatesNothing pins that (*Runner).log
// does not allocate when Logger is unset. log runs once per file in
// the workspace walk (Runner.checkFile / lintFile), so a fresh
// &vlog.Logger{} on every call — the common case, since -v is off by
// default — is one avoidable allocation per file. A single shared
// disabled-logger value is safe to return in its place: every caller
// only reads Enabled or calls Printf, which no-ops before touching
// the logger's mutex when Enabled is false, so nothing ever mutates
// the shared instance.
func TestRunner_log_NoLoggerAllocatesNothing(t *testing.T) {
	r := &Runner{}

	// Escapes to a package-visible sink so the compiler can't prove
	// the result is unused and elide the allocation the way it would
	// for a discarded `_ = r.log()` — real call sites (runner.go's
	// `r.log().Printf(...)`, `sink := r.log()`) keep the pointer live
	// past the call the same way.
	sink := make([]*vlog.Logger, 0, 200)
	allocs := testing.AllocsPerRun(200, func() {
		sink = append(sink[:0], r.log())
	})
	if allocs != 0 {
		t.Fatalf("(*Runner).log() allocated %v times per call with Logger unset, want 0", allocs)
	}
}

// TestRunner_log_ReturnsDisabledLogger pins the documented contract:
// with no Logger configured, log() returns a non-nil, disabled
// logger so callers never need a nil check.
func TestRunner_log_ReturnsDisabledLogger(t *testing.T) {
	r := &Runner{}
	l := r.log()
	if l == nil {
		t.Fatal("(*Runner).log() returned nil, want a disabled logger")
	}
	if l.Enabled {
		t.Fatal("(*Runner).log() returned an enabled logger with no Logger configured")
	}
}

// TestRunner_log_PrefersConfiguredLogger pins that an explicitly set
// Logger is returned unchanged rather than the shared disabled
// sentinel.
func TestRunner_log_PrefersConfiguredLogger(t *testing.T) {
	configured := &vlog.Logger{Enabled: true}
	r := &Runner{Logger: configured}
	if got := r.log(); got != configured {
		t.Fatalf("(*Runner).log() = %p, want the configured logger %p", got, configured)
	}
}
