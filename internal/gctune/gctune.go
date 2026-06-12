// Package gctune centralizes mdsmith's process-level GC policy so every
// entry point that does short-lived batch work applies the same target
// from one place: the native CLI's check/fix commands and the
// WebAssembly engine. The LSP server and the embedded pkg/mdsmith
// library deliberately do NOT call it, so long-running servers and host
// embedders keep Go's default policy.
package gctune

import (
	"os"
	"runtime/debug"
)

// BatchPercent is the GC target (the GOGC equivalent) for short-lived
// batch work. Go's default GOGC=100 collects every time the live heap
// doubles; on a workspace walk that means re-scanning the pointer-rich
// AST set constantly — profiling attributed ~40% of executed
// instructions to GC. A batch run is about to exit, so a laxer target
// trades a little peak memory for markedly less wall time. GOGC=off is
// slower than either tuned value because an unbounded heap wrecks
// cache locality.
//
// The sweet spot tracks allocation volume, so re-measure it after any
// large allocation change: 400 was measured before the parse-arena
// pool landed; with pooling cutting allocation roughly in half, 300
// measured both faster and markedly stabler (lower run-to-run
// variance) on the parity corpus.
const BatchPercent = 300

// Target returns the GC percent ApplyBatch should set, or -1 to leave
// the runtime default untouched when the user pinned GOGC explicitly (an
// empty value is treated as unset). Split out as a pure function so the
// decision is unit-testable without mutating process-global GC state.
func Target(gogcEnv string) int {
	if gogcEnv == "" {
		return BatchPercent
	}
	return -1
}

// ApplyBatch raises the GC target to BatchPercent unless the user pinned
// GOGC. Call it once at the start of a batch entry point. It reads GOGC
// programmatically and applies the default via debug.SetGCPercent, so it
// works without any environment variable being set; an explicit GOGC is
// the opt-out.
func ApplyBatch() {
	if p := Target(os.Getenv("GOGC")); p >= 0 {
		debug.SetGCPercent(p)
	}
}
