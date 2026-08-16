package fix

import (
	"testing"

	vlog "github.com/jeduden/mdsmith/internal/log"
)

// TestFixer_log_NoLoggerAllocatesNothing pins that (*Fixer).log does
// not allocate when Logger is unset — the common case, since -v is
// off by default. log runs at least once per file per fix pass
// (fix.go's `f.log().Printf("fix: pass %d on %s", ...)`, up to 10
// passes), mirroring internal/engine's identical (*Runner).log fix:
// a fresh &vlog.Logger{} per call there was one avoidable allocation
// per file in the workspace walk.
func TestFixer_log_NoLoggerAllocatesNothing(t *testing.T) {
	f := &Fixer{}

	// Escapes to a package-visible sink so the compiler can't prove
	// the result is unused and elide the allocation the way it would
	// for a discarded `_ = f.log()` — real call sites keep the
	// pointer live past the call by calling Printf on it.
	sink := make([]*vlog.Logger, 0, 200)
	allocs := testing.AllocsPerRun(200, func() {
		sink = append(sink[:0], f.log())
	})
	if allocs != 0 {
		t.Fatalf("(*Fixer).log() allocated %v times per call with Logger unset, want 0", allocs)
	}
}

// TestFixer_log_ReturnsDisabledLogger pins the documented contract:
// with no Logger configured, log() returns a non-nil, disabled
// logger so callers never need a nil check.
func TestFixer_log_ReturnsDisabledLogger(t *testing.T) {
	f := &Fixer{}
	l := f.log()
	if l == nil {
		t.Fatal("(*Fixer).log() returned nil, want a disabled logger")
	}
	if l.Enabled {
		t.Fatal("(*Fixer).log() returned an enabled logger with no Logger configured")
	}
}

// TestFixer_log_PrefersConfiguredLogger pins that an explicitly set
// Logger is returned unchanged rather than the shared disabled
// sentinel.
func TestFixer_log_PrefersConfiguredLogger(t *testing.T) {
	configured := &vlog.Logger{Enabled: true}
	f := &Fixer{Logger: configured}
	if got := f.log(); got != configured {
		t.Fatalf("(*Fixer).log() = %p, want the configured logger %p", got, configured)
	}
}
