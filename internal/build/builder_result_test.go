package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestBuildWithResult_DoneContextStagesNothing(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithTimeout(context.Background(), -time.Second)
	defer stop()

	for name, tc := range map[string]struct {
		ctx      context.Context
		ctxErr   error
		timedOut bool
	}{
		"cancel":   {cancelled, context.Canceled, false},
		"deadline": {expired, context.DeadlineExceeded, true},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			// An earlier run of the same action left its log: a run
			// refused before start must not truncate it.
			logPath := logPathFor(root, "sha256-x")
			require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
			require.NoError(t, os.WriteFile(logPath, []byte("old"), 0o644))

			b := NewCustomBuilder(map[string]RecipeSpec{"echo": recipeCmd("echo hi")})
			res := b.BuildWithResult(tc.ctx, Target{
				Recipe:  "echo",
				Root:    root,
				Outputs: []string{"out.txt"},
			}, Options{ActionID: "sha256-x", LogRoot: root})

			require.ErrorIs(t, res.Err, ErrNotStarted)
			assert.ErrorIs(t, res.Err, tc.ctxErr)
			assert.ErrorContains(t, res.Err, `recipe "echo" failed`)
			assert.Equal(t, -1, res.ExitCode)
			assert.Equal(t, tc.timedOut, res.TimedOut)
			assert.Empty(t, res.LogPath)
			got, err := os.ReadFile(logPath)
			require.NoError(t, err)
			assert.Equal(t, "old", string(got))
			assert.NoDirExists(t, filepath.Join(root, stagingRootRel))
		})
	}
}

func TestNotStartedResult(t *testing.T) {
	res := notStartedResult("gen", context.Canceled)
	assert.Equal(t, -1, res.ExitCode)
	assert.False(t, res.TimedOut)
	assert.ErrorIs(t, res.Err, ErrNotStarted)
	assert.ErrorIs(t, res.Err, context.Canceled)
	assert.EqualError(t, res.Err, `recipe "gen" failed: `+NotStartedError(context.Canceled).Error())

	res = notStartedResult("gen", context.DeadlineExceeded)
	assert.True(t, res.TimedOut)
	assert.ErrorIs(t, res.Err, context.DeadlineExceeded)
}
