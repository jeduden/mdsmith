package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildWithResult_LogSetupError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix path trick not applicable on Windows")
	}
	root := t.TempDir()
	// Block log dir: place a file at .mdsmith/build-logs so MkdirAll fails.
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith", "build-logs"), []byte("block"), 0o644))

	b := NewCustomBuilder(map[string]RecipeSpec{"echo": recipeCmd("echo hi")})
	res := b.BuildWithResult(context.Background(), Target{
		Recipe:  "echo",
		Root:    root,
		Outputs: []string{"out.txt"},
	}, Options{ActionID: "sha256-x", LogRoot: root})

	// The recipe would fail too (no process under js/wasm, no output
	// natively), so pin the error to the log setup.
	require.ErrorContains(t, res.Err, "creating build-logs dir")
}
