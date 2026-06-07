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
// Plan 218 removed CUE (95 packages) and protobuf from the artifact by
// build-tagging a CUE-free engine path behind //go:build wasm: the
// CUE-backed schema/query/cuetemplate validation, the {field}
// interpolation CUE-path parser, and the on-disk index/githook helpers
// all have native and WASM variants now. That drops the artifact from
// ~38 MB to ~10.5 MB raw (~2.7 MB gzipped), under the plan-215 budget
// of 18 MB this plan finally reaches.
//
// The ceilings below are the plan budget (raw) plus a comfortable
// gzip ceiling; the test fails only if the artifact GROWS past them,
// so an accidental re-introduction of CUE/protobuf (or any large
// dependency) is caught in CI.
const (
	maxWASMRawBytes  = 18 * 1024 * 1024 // 18 MiB (plan-215/218 budget)
	maxWASMGzipBytes = 5 * 1024 * 1024  // 5 MiB
)

// Size budget for the tinygo WASM artifact (plan 218). tinygo produces
// a much smaller binary (~3 MB) than the standard Go toolchain; the
// plan budget is 8 MB. Built with the same flags as build.sh's tinygo
// target, including -stack-size=1MB (the engine's package init
// overflows tinygo's default 64 KB stack — see build.sh).
const maxTinygoWASMRawBytes = 8 * 1024 * 1024 // 8 MiB

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

// TestTinygoWASMArtifactSizeBudget builds the WASM artifact with the
// tinygo toolchain using the same flags as build.sh's tinygo target
// and asserts it stays within the plan-218 budget. tinygo is not on
// every CI host, so the test skips (rather than fails) when the
// toolchain is unavailable — mirroring the smoke test's node skip.
func TestTinygoWASMArtifactSizeBudget(t *testing.T) {
	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		t.Skip("tinygo not on PATH; skipping tinygo size budget test")
	}

	out := filepath.Join(t.TempDir(), "mdsmith.wasm")
	// Mirror build.sh's tinygo target: -no-debug strips DWARF,
	// -stack-size=1MB clears the engine's init-time stack overflow.
	cmd := exec.Command(tinygo, "build", "-target", "wasm",
		"-no-debug", "-stack-size=1MB", "-o", out, ".")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building tinygo wasm artifact: %v\n%s", err, b)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading tinygo wasm artifact: %v", err)
	}
	raw := len(data)

	const mib = 1024 * 1024
	t.Logf("tinygo wasm artifact: raw=%d bytes (%.1f MiB)", raw, float64(raw)/mib)

	if raw > maxTinygoWASMRawBytes {
		t.Errorf("tinygo wasm raw size %d bytes exceeds budget %d (%.1f MiB > %.1f MiB)",
			raw, maxTinygoWASMRawBytes, float64(raw)/mib, float64(maxTinygoWASMRawBytes)/mib)
	}
}
