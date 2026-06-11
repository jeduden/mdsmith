package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Size budgets for the shipping standard-Go WASM artifact, built with
// the same -trimpath -ldflags="-s -w" flags as build.sh.
//
// With cuelang.org/go removed (plan 218/240 — the in-house cue/cuelite
// engine replaced it), the artifact dropped from ~37.9 MB raw to the
// sizes below. The plan-215 standard-Go target was ≤ 18 MB; the
// artifact now clears it comfortably. These ceilings are REGRESSION
// GUARDS set just above the measured size, so an accidental dependency
// bloat is caught in CI.
//
// Measured (Go 1.25, stripped): ~11.2 MB raw / ~2.8 MB gzipped. The
// ceilings leave headroom for toolchain-version drift while staying
// under the 18 MB plan-215 budget.
const (
	maxWASMRawBytes  = 14 * 1024 * 1024 // 14 MiB (< 18 MiB plan-215 budget)
	maxWASMGzipBytes = 4 * 1024 * 1024  // 4 MiB
)

// TestWASMArtifactSizeBudget builds the shipping WASM artifact with the
// same flags as build.sh and asserts it stays within the
// regression-guard ceilings. It needs only the standard Go wasm
// toolchain (no node), so it gates on every host that runs the wasm
// job — and in the main test job's `go test ./...` too.
func TestWASMArtifactSizeBudget(t *testing.T) {
	out := filepath.Join(t.TempDir(), "mdsmith.wasm")
	// Mirror build.sh's `go` target: -trimpath for reproducibility,
	// -ldflags="-s -w" to strip the symbol table and DWARF.
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", out, ".")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building wasm artifact: %v\n%s", err, b)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading wasm artifact: %v", err)
	}
	raw := len(data)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	gz := buf.Len()

	const mib = 1024 * 1024
	t.Logf("wasm artifact: raw=%d bytes (%.1f MiB), gzip=%d bytes (%.1f MiB)",
		raw, float64(raw)/mib, gz, float64(gz)/mib)

	if raw > maxWASMRawBytes {
		t.Errorf("wasm raw size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			raw, maxWASMRawBytes, float64(raw)/mib, float64(maxWASMRawBytes)/mib)
	}
	if gz > maxWASMGzipBytes {
		t.Errorf("wasm gzip size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			gz, maxWASMGzipBytes, float64(gz)/mib, float64(maxWASMGzipBytes)/mib)
	}
}

// maxTinyGoWASMBytes is the plan-215 tinygo budget. Removing cuelang.org/go
// and swapping the sync.Map.CompareAndDelete lever for a mutex-guarded map
// (plan 240) cleared two earlier tinygo walls, but the build does NOT yet
// succeed: tinygo's wasm target leaves os.Chmod, os.SameFile, and
// os.Symlink/filepath.EvalSymlinks undefined, and pkg/mdsmith reaches all
// three transitively (internal/schema atomic index writes, internal/fix,
// internal/githooks, and the cross-file rule packages). So the 8 MiB budget
// is a TARGET, not a verified ceiling.
const maxTinyGoWASMBytes = 8 * 1024 * 1024 // 8 MiB

// TestTinyGoWASMArtifactSizeBudget attempts the tinygo wasm build and records
// the result honestly. The build currently fails on tinygo-unimplemented
// standard-library calls (see maxTinyGoWASMBytes), so the test SKIPS with the
// real failure recorded rather than passing — making the tinygo build succeed
// requires build-tagging those os calls out of the wasm graph, which is
// tracked follow-up work (plan 240). The test does not fail the suite on the
// known incompatibility: that would block every PR on work not yet done; it
// surfaces the failure in the test log instead. It also SKIPS when tinygo is
// not installed (the offline dev container and the standard-Go test job).
//
// When the build is fixed, replace the skip with the size assertion against
// maxTinyGoWASMBytes.
func TestTinyGoWASMArtifactSizeBudget(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not installed; the tinygo build is not yet verifiable here")
	}
	out := filepath.Join(t.TempDir(), "mdsmith-tinygo.wasm")
	cmd := exec.Command("tinygo", "build", "-target", "wasm", "-o", out, ".")
	b, err := cmd.CombinedOutput()
	if err != nil {
		// Known incompatibility: tinygo's wasm target does not implement the os
		// calls pkg/mdsmith reaches. Record it and skip rather than fail the
		// suite on work that is not yet done.
		t.Skipf("tinygo wasm build is not yet supported (known os.* gaps); build output:\n%s", b)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading tinygo wasm artifact: %v", err)
	}
	const mib = 1024 * 1024
	t.Logf("tinygo wasm artifact: raw=%d bytes (%.1f MiB)", len(data), float64(len(data))/mib)
	if len(data) > maxTinyGoWASMBytes {
		t.Errorf("tinygo wasm raw size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			len(data), maxTinyGoWASMBytes, float64(len(data))/mib, float64(maxTinyGoWASMBytes)/mib)
	}
}
