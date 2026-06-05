package main

import (
	"os"
	"runtime/debug"
)

// batchGCPercent is the GC target for the short-lived `check` and `fix`
// batch commands. The default GOGC=100 runs the collector every time the
// live heap doubles; on a workspace walk that means re-scanning the
// pointer-rich AST set constantly (profiling attributes ~40% of executed
// instructions to GC). A batch process is about to exit, so a laxer
// target trades a little peak memory — already bounded by the 512 MB
// SetMemoryLimit in run() — for markedly less wall time. 400 was the
// measured sweet spot on the parity corpus; GOGC=off was slower because
// an unbounded heap wrecks cache locality.
const batchGCPercent = 400

// batchGCTarget returns the GC percent to apply for batch commands, or
// -1 to leave the runtime default in place when the user pinned GOGC
// explicitly (an empty value is treated as unset).
func batchGCTarget(gogcEnv string) int {
	if gogcEnv == "" {
		return batchGCPercent
	}
	return -1
}

// tuneGCForBatch raises the GC target for the batch check/fix commands
// unless the user pinned GOGC. The LSP server and the public library
// never call this, so long-running and embedded callers keep the
// runtime default.
func tuneGCForBatch() {
	if p := batchGCTarget(os.Getenv("GOGC")); p >= 0 {
		debug.SetGCPercent(p)
	}
}
