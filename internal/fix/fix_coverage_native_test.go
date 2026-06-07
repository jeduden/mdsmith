//go:build !wasm

package fix

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAtomicWriteFile_ChmodErrorSurfaces drives the chmodFile error
// branch in atomicWriteFile by injecting a failing chmodFileFn.
func TestAtomicWriteFile_ChmodErrorSurfaces(t *testing.T) {
	orig := chmodFileFn
	t.Cleanup(func() { chmodFileFn = orig })
	chmodFileFn = func(_ string, _ os.FileMode) error {
		return os.ErrPermission
	}

	dir := t.TempDir()
	err := atomicWriteFile(filepath.Join(dir, "out.txt"), []byte("data"), 0o644)
	require.ErrorIs(t, err, os.ErrPermission)
}
