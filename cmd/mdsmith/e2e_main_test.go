//go:build !wasm

package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMain builds the mdsmith binary once for every e2e test. It needs
// os/exec, so it is left out of a GOOS=js GOARCH=wasm test build; there
// the default runner runs only the wasm-only unit tests
// (mdsmith-release test-js-wasm), which never call binaryPath.
func TestMain(m *testing.M) {
	// Build the binary once for all e2e tests.
	// go test runs from the package directory (cmd/mdsmith/),
	// so "go build ." builds the main package in this directory.
	tmp, err := os.MkdirTemp("", "mdsmith-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	// Create a shared directory for coverage data from all e2e runs.
	coverDir, err = os.MkdirTemp("", "mdsmith-e2e-cover-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create cover dir: %v\n", err)
		_ = os.RemoveAll(tmp)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmp, "mdsmith")
	cmd := exec.Command("go", "build", "-cover", "-covermode=atomic", "-o", binaryPath, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build binary: %v\n", err)
		_ = os.RemoveAll(tmp)
		_ = os.RemoveAll(coverDir)
		os.Exit(1)
	}

	// Create an isolated working directory with a .git marker and minimal
	// config to prevent runBinary from inheriting the repo root's config.
	isolatedCWD = createIsolatedCWD(tmp, coverDir)

	code := m.Run()
	_ = os.RemoveAll(isolatedCWD)

	// Merge e2e coverage data into a text profile if E2E_COVERDIR is
	// set by the caller, so it can be combined with unit-test coverage.
	if outDir := os.Getenv("E2E_COVERDIR"); outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "E2E_COVERDIR: cannot create %s: %v\n", outDir, err)
			code = 1
		} else {
			mergeCmd := exec.Command("go", "tool", "covdata", "textfmt",
				"-i="+coverDir, "-o="+filepath.Join(outDir, "e2e_coverage.txt"))
			mergeCmd.Stderr = os.Stderr
			if err := mergeCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "E2E_COVERDIR: failed to export coverage: %v\n", err)
				code = 1
			}
		}
	}

	_ = os.RemoveAll(tmp)
	_ = os.RemoveAll(coverDir)
	os.Exit(code)
}

// createIsolatedCWD creates a temp directory with .git and .mdsmith.yml
// to prevent config discovery from walking up to the repo root.
func createIsolatedCWD(cleanupDirs ...string) string {
	dir, err := os.MkdirTemp("", "mdsmith-e2e-cwd-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create isolated CWD: %v\n", err)
		for _, d := range cleanupDirs {
			_ = os.RemoveAll(d)
		}
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create .git marker: %v\n", err)
		_ = os.RemoveAll(dir)
		for _, d := range cleanupDirs {
			_ = os.RemoveAll(d)
		}
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write .mdsmith.yml: %v\n", err)
		_ = os.RemoveAll(dir)
		for _, d := range cleanupDirs {
			_ = os.RemoveAll(d)
		}
		os.Exit(1)
	}
	return dir
}
