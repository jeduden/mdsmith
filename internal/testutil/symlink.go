// Package testutil holds small helpers shared across test
// binaries. It is intended for use only from *_test.go files.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// SkipIfSymlinkUnsupported skips the calling test when the host
// cannot create symbolic links. This typically happens on Windows
// without Developer Mode or elevated privileges, and in some
// sandboxed CI environments.
//
// Both a file symlink and a directory symlink are probed: on
// Windows these use different code paths and can have different
// privilege requirements, so a test that assumes one works because
// the other does would fail instead of being skipped. Every test
// in the repo that creates a symlink may create either kind, so
// we gate conservatively on the weaker capability.
func SkipIfSymlinkUnsupported(t *testing.T) {
	t.Helper()
	probe := t.TempDir()

	fileTarget := filepath.Join(probe, "f")
	if err := os.WriteFile(fileTarget, nil, 0o644); err != nil {
		t.Skipf("cannot create probe file: %v", err)
	}
	if err := os.Symlink(fileTarget, filepath.Join(probe, "flink")); err != nil {
		t.Skipf("file symlinks not supported on this host: %v", err)
	}

	dirTarget := filepath.Join(probe, "d")
	if err := os.MkdirAll(dirTarget, 0o755); err != nil {
		t.Skipf("cannot create probe directory: %v", err)
	}
	if err := os.Symlink(dirTarget, filepath.Join(probe, "dlink")); err != nil {
		t.Skipf("directory symlinks not supported on this host: %v", err)
	}
}
