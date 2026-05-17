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
		f, err := os.Create(cpuPath)
		if err != nil {
			_, _ = fmt.Fprintf(diag, "profiling: cpu profile: %v\n", err)
		} else if err := pprof.StartCPUProfile(f); err != nil {
			_, _ = fmt.Fprintf(diag, "profiling: cpu profile: %v\n", err)
			_ = f.Close()
		} else {
			cpuFile = f
		}
	}

	return func() {
		if cpuFile != nil {
			pprof.StopCPUProfile()
			_ = cpuFile.Close()
		}
		if memPath != "" {
			f, err := os.Create(memPath)
			if err != nil {
				_, _ = fmt.Fprintf(diag, "profiling: mem profile: %v\n", err)
				return
			}
			runtime.GC() // materialize live heap before the dump
			if err := pprof.WriteHeapProfile(f); err != nil {
				_, _ = fmt.Fprintf(diag, "profiling: mem profile: %v\n", err)
			}
			_ = f.Close()
		}
	}
}
