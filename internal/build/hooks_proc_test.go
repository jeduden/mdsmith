//go:build unix || windows

package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookEntry builds a HookEntry that calls `echo` (a real binary available
// everywhere) so we can test success without a custom binary.
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

func TestRunHooks_SingleSuccess(t *testing.T) {
	var w bytes.Buffer
	result := RunHooks(context.Background(), []HookEntry{echoEntry("greet", "hi")}, t.TempDir(), &w)
	assert.Nil(t, result)
	assert.Contains(t, w.String(), "greet: OK")
}

func TestRunHooks_MultipleSuccess(t *testing.T) {
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
	var w bytes.Buffer
	result := RunAfterHooks(context.Background(),
		[]HookEntry{echoEntry("a", "1"), echoEntry("b", "2")},
		t.TempDir(), &w)
	assert.Nil(t, result)
}

func TestRunAfterHooks_FailContinues(t *testing.T) {
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
	var w bytes.Buffer
	hooks := []HookEntry{echoEntry("x", "hello")}
	result := RunHooks(context.Background(), hooks, t.TempDir(), &w)
	assert.Nil(t, result)
	assert.NotContains(t, w.String(), "FAIL")
}

func TestRunAfterHooks_UnnamedHook_UsesFirstToken(t *testing.T) {
	var w bytes.Buffer
	h := echoEntry("echo", "hello")
	h.Name = "" // force the name-from-token path
	result := RunAfterHooks(context.Background(), []HookEntry{h}, t.TempDir(), &w)
	assert.Nil(t, result)
	assert.Contains(t, w.String(), "hook echo: running")
}
