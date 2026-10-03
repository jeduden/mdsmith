//go:build !js && !wasip1

package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoEntry builds a HookEntry that calls the `echo` binary so we can
// test success without a custom binary.
func echoEntry(name, msg string) HookEntry {
	return HookEntry{
		Tokens: []string{"echo", msg},
		Name:   name,
	}
}

// sentinelEntry returns a HookEntry that touches a file in dir.
func sentinelEntry(t *testing.T, dir, name, sentinel string) HookEntry {
	t.Helper()
	return HookEntry{
		Tokens: []string{"touch", filepath.Join(dir, sentinel)},
		Name:   name,
	}
}

// skipWithoutUnixTools skips on Windows, where the hooks below cannot
// run: `echo` is a cmd builtin with no binary, and `touch` is absent.
func skipWithoutUnixTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("echo and touch are not binaries on Windows")
	}
}

func TestRunHooks_SingleSuccess(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	var w bytes.Buffer
	result := RunHooks(context.Background(), []HookEntry{echoEntry("greet", "hi")}, t.TempDir(), &w)
	assert.Nil(t, result)
	assert.Contains(t, w.String(), "greet: OK")
}

func TestRunHooks_MultipleSuccess(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	dir := t.TempDir()
	hooks := []HookEntry{
		sentinelEntry(t, dir, "a", "a.txt"),
		sentinelEntry(t, dir, "b", "b.txt"),
	}
	var w bytes.Buffer
	result := RunHooks(context.Background(), hooks, dir, &w)
	assert.Nil(t, result)
	_, errA := os.Stat(filepath.Join(dir, "a.txt"))
	_, errB := os.Stat(filepath.Join(dir, "b.txt"))
	assert.NoError(t, errA, "sentinel a.txt must exist")
	assert.NoError(t, errB, "sentinel b.txt must exist")
}

func TestRunHooks_UsesRootAsWorkDir(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	dir := t.TempDir()
	// The hook creates a file named "ok" — relative to cwd (= root).
	hook := HookEntry{Tokens: []string{"touch", "ok"}, Name: "sentinel"}
	var w bytes.Buffer
	result := RunHooks(context.Background(), []HookEntry{hook}, dir, &w)
	assert.Nil(t, result)
	_, err := os.Stat(filepath.Join(dir, "ok"))
	assert.NoError(t, err, "sentinel must be created in root")
}

func TestRunAfterHooks_AllSucceed_ReturnsNil(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	var w bytes.Buffer
	result := RunAfterHooks(context.Background(),
		[]HookEntry{echoEntry("a", "1"), echoEntry("b", "2")},
		t.TempDir(), &w)
	assert.Nil(t, result)
}

func TestRunAfterHooks_FailContinues(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "second-ran")
	hooks := []HookEntry{
		failEntry("first"),
		{Tokens: []string{"touch", sentinel}, Name: "second"},
	}
	var w bytes.Buffer
	result := RunAfterHooks(context.Background(), hooks, dir, &w)
	require.NotNil(t, result, "first failure must be returned")
	// Second hook must still have run.
	_, err := os.Stat(sentinel)
	assert.NoError(t, err, "second hook should have run despite first failure")
}

func TestRunHooks_OutputLines(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	var w bytes.Buffer
	hooks := []HookEntry{
		{Tokens: []string{"echo", "hello"}, Name: "greet"},
	}
	RunHooks(context.Background(), hooks, t.TempDir(), &w)
	out := w.String()
	assert.Contains(t, out, "hook greet: running")
	assert.Contains(t, out, "hook greet: OK")
}

// Regression: zero-exit hook must not produce a FAIL line.
func TestRunHooks_SuccessNoFailLine(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	var w bytes.Buffer
	hooks := []HookEntry{echoEntry("x", "hello")}
	result := RunHooks(context.Background(), hooks, t.TempDir(), &w)
	assert.Nil(t, result)
	assert.NotContains(t, w.String(), "FAIL")
}

func TestRunAfterHooks_UnnamedHook_UsesFirstToken(t *testing.T) {
	skipOnPlan9(t)
	skipWithoutUnixTools(t)
	var w bytes.Buffer
	h := echoEntry("echo", "hello")
	h.Name = "" // force the name-from-token path
	result := RunAfterHooks(context.Background(), []HookEntry{h}, t.TempDir(), &w)
	assert.Nil(t, result)
	assert.Contains(t, w.String(), "hook echo: running")
}

// TestRunHook_ExitCodePreserved runs a script that exits 42, so the code
// can only come from exec.ExitError. A start failure, or a dropped
// ExitCode branch, would report the default code 1 and fail this test.
func TestRunHook_ExitCodePreserved(t *testing.T) {
	skipOnPlan9(t)
	tokens := []string{"cmd", "/c", "exit 42"}
	if runtime.GOOS != "windows" {
		tokens = []string{writeScript(t, t.TempDir(), "exit42.sh", `exit 42`)}
	}
	result := runHook(context.Background(), tokens, t.TempDir())
	require.NotNil(t, result)
	assert.Equal(t, 42, result.ExitCode)
}

// TestRunHook_SignalKilled_NormalizesExitCode exercises the code < 0 branch:
// a process killed by a signal yields ExitCode() == -1, which runHook normalizes to 1.
// Only meaningful on Unix (Windows processes don't signal-kill the same way).
func TestRunHook_SignalKilled_NormalizesExitCode(t *testing.T) {
	skipOnPlan9(t)
	if runtime.GOOS == "windows" {
		t.Skip("signal kill not available on windows")
	}
	// `sh -c 'kill -9 $$'` kills the shell with SIGKILL, giving exit code -1.
	result := runHook(context.Background(), []string{"sh", "-c", "kill -9 $$"}, t.TempDir())
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ExitCode, "negative signal exit code must be normalized to 1")
}
