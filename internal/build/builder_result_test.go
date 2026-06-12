package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWithResult_CapturesArgvCwdAndLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeScript(t, bindir, "emit.sh",
		`echo hello-out; echo hello-err 1>&2; printf payload > "$1"`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"emit": recipeCmd(script + " {outputs}"),
	})

	res := b.BuildWithResult(context.Background(), Target{
		Recipe:  "emit",
		Root:    root,
		Outputs: []string{"out.txt"},
	}, Options{ActionID: "sha256-abc", LogRoot: root})

	require.NoError(t, res.Err)
	assert.Equal(t, 0, res.ExitCode)
	assert.Equal(t, script, res.Argv[0])
	assert.NotEmpty(t, res.Cwd)
	assert.Greater(t, res.Duration, time.Duration(0))
	assert.Equal(t, logPathFor(root, "sha256-abc"), res.LogPath)

	logData, err := os.ReadFile(res.LogPath)
	require.NoError(t, err)
	assert.Contains(t, string(logData), "[stdout] hello-out")
	assert.Contains(t, string(logData), "[stderr] hello-err")
	assert.Contains(t, res.StdoutTail, "hello-out")
	assert.Contains(t, res.StderrTail, "hello-err")
}

func TestBuildWithResult_FailingRecipeReportsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeScript(t, bindir, "boom.sh", `echo boom 1>&2; exit 7`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"boom": recipeCmd(script + " {outputs}"),
	})

	res := b.BuildWithResult(context.Background(), Target{
		Recipe:  "boom",
		Root:    root,
		Outputs: []string{"out.txt"},
	}, Options{ActionID: "sha256-boom", LogRoot: root})

	require.Error(t, res.Err)
	assert.Equal(t, 7, res.ExitCode)
	assert.Contains(t, res.StderrTail, "boom")
	assert.NoFileExists(t, filepath.Join(root, "out.txt"))
}

func TestBuildWithResult_LiveSinkForwardsLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeScript(t, bindir, "stream.sh",
		`echo line-a; echo line-b; printf x > "$1"`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"stream": recipeCmd(script + " {outputs}"),
	})

	var sink strings.Builder
	res := b.BuildWithResult(context.Background(), Target{
		Recipe:  "stream",
		Root:    root,
		Outputs: []string{"out.txt"},
	}, Options{ActionID: "sha256-s", LogRoot: root, LiveSink: &sink, TargetName: "out.txt"})

	require.NoError(t, res.Err)
	assert.Contains(t, sink.String(), "[out.txt] line-a")
	assert.Contains(t, sink.String(), "[out.txt] line-b")
}

func TestBuildWithResult_TimeoutFlagSet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeScript(t, bindir, "hang.sh", `echo starting; sleep 30`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"hang": recipeCmd(script + " {outputs}"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	res := b.BuildWithResult(ctx, Target{
		Recipe:  "hang",
		Root:    root,
		Outputs: []string{"out.txt"},
	}, Options{ActionID: "sha256-h", LogRoot: root})

	require.Error(t, res.Err)
	assert.True(t, res.TimedOut)
}

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

	require.Error(t, res.Err)
}
