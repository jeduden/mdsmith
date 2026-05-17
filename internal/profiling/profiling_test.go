package profiling

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}

func TestStart_NoEnv_IsNoOp(t *testing.T) {
	var diag bytes.Buffer
	stop := start("", "", &diag)
	if stop == nil {
		t.Fatal("stop must never be nil")
	}
	stop() // must not panic and must write nothing
	if diag.Len() != 0 {
		t.Fatalf("expected no diagnostics, got %q", diag.String())
	}
}

func TestStart_MemProfile_WritesNonEmpty(t *testing.T) {
	mem := filepath.Join(t.TempDir(), "mem.out")
	var diag bytes.Buffer
	stop := start("", mem, &diag)
	sink := make([]byte, 1<<20) // force a live allocation
	_ = sink
	stop()
	if got := sizeOf(t, mem); got == 0 {
		t.Fatalf("heap profile is empty")
	}
	if diag.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %q", diag.String())
	}
}

func TestStart_CPUProfile_WritesNonEmpty(t *testing.T) {
	cpu := filepath.Join(t.TempDir(), "cpu.out")
	var diag bytes.Buffer
	stop := start(cpu, "", &diag)
	// Burn a little CPU so the profile has at least a header.
	x := 0
	for i := 0; i < 5_000_000; i++ {
		x += i % 7
	}
	_ = x
	stop()
	if got := sizeOf(t, cpu); got == 0 {
		t.Fatalf("cpu profile is empty")
	}
	if diag.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %q", diag.String())
	}
}

func TestStart_CPUCreateError_ReportsAndContinues(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "nope", "cpu.out") // missing dir
	var diag bytes.Buffer
	stop := start(bad, "", &diag)
	stop() // no cpu profiling was started; must be a safe no-op
	if !strings.Contains(diag.String(), "cpu profile") {
		t.Fatalf("expected cpu-profile diagnostic, got %q", diag.String())
	}
}

func TestStart_MemCreateError_ReportsOnStop(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "nope", "mem.out")
	var diag bytes.Buffer
	stop := start("", bad, &diag)
	stop()
	if !strings.Contains(diag.String(), "mem profile") {
		t.Fatalf("expected mem-profile diagnostic, got %q", diag.String())
	}
}

func TestStart_EnvWrapper_NonNil(t *testing.T) {
	t.Setenv("MDSMITH_CPUPROFILE", "")
	t.Setenv("MDSMITH_MEMPROFILE", "")
	stop := Start()
	if stop == nil {
		t.Fatal("Start must return a non-nil stop")
	}
	stop()
}
