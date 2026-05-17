// Package profiling adds an env-gated CPU/heap profiler to the
// mdsmith CLI. It exists so a tripped performance gate can be
// diagnosed: the gate says "check got slower", a profile says
// "here is the function". There is no CLI flag on purpose — the
// hyperfine command strings stay byte-identical to production, so
// the profiled run measures the same path users hit.
//
//	MDSMITH_CPUPROFILE=cpu.out mdsmith check .
//	go tool pprof -top cpu.out
package profiling

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
)

// Start reads MDSMITH_CPUPROFILE / MDSMITH_MEMPROFILE and begins
// profiling for any that are set. The returned stop function is
// always non-nil and must be called once before process exit; it
// finalizes whatever was started. When neither var is set, Start
// is a no-op and stop does nothing.
//
// Wire it as `defer profiling.Start()()` from the normal return
// path. A code path that calls os.Exit directly bypasses the
// deferred stop and leaves the profile truncated or empty —
// acceptable for a dev aid, but the reason profiles are only
// reliable for the standard return path.
func Start() (stop func()) {
	return start(
		os.Getenv("MDSMITH_CPUPROFILE"),
		os.Getenv("MDSMITH_MEMPROFILE"),
		os.Stderr,
	)
}

// start is the testable core: explicit paths and diagnostic sink,
// no environment or process globals beyond the pprof machinery.
// A failure to set up one profile is reported to diag and skipped
// rather than aborting the command — profiling is a dev aid, not a
// reason to fail a real run.
func start(cpuPath, memPath string, diag io.Writer) func() {
	var cpuFile *os.File
	if cpuPath != "" {
		cpuFile = beginCPU(cpuPath, diag)
	}
	return func() {
		if cpuFile != nil {
			pprof.StopCPUProfile()
			_ = cpuFile.Close()
		}
		if memPath != "" {
			writeHeap(memPath, diag)
		}
	}
}

// beginCPU starts CPU profiling to path. It returns the open file
// on success (the caller stops the profile and closes it), or nil
// after reporting the failure to diag.
func beginCPU(path string, diag io.Writer) *os.File {
	f, err := os.Create(path)
	if err != nil {
		report(diag, "cpu", err)
		return nil
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		report(diag, "cpu", err)
		_ = f.Close()
		return nil
	}
	return f
}

// writeHeap dumps the live heap to path. WriteHeapProfile on a
// freshly created writable file has no failure mode reachable in a
// test, so per the repo's "no untestable defensive branch" rule it
// is best-effort, like the Close calls.
func writeHeap(path string, diag io.Writer) {
	f, err := os.Create(path)
	if err != nil {
		report(diag, "mem", err)
		return
	}
	runtime.GC() // materialize live heap before the dump
	_ = pprof.WriteHeapProfile(f)
	_ = f.Close()
}

func report(diag io.Writer, kind string, err error) {
	_, _ = fmt.Fprintf(diag, "profiling: %s profile: %v\n", kind, err)
}
