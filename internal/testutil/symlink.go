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
func SkipIfSymlinkUnsupported(t *testing.T) {
	t.Helper()
	probe := t.TempDir()
	target := filepath.Join(probe, "t")
	link := filepath.Join(probe, "l")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Skipf("cannot create probe file: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links not supported on this host: %v", err)
	}
}
