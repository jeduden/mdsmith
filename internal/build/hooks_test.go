package build

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TokenizeHook ---

func TestTokenizeHook_NoParams(t *testing.T) {
	tokens := TokenizeHook("make dev-server-start", nil)
	assert.Equal(t, []string{"make", "dev-server-start"}, tokens)
}

func TestTokenizeHook_WithParams(t *testing.T) {
	tokens := TokenizeHook("scripts/wait {port}", map[string]string{"port": "3000"})
	assert.Equal(t, []string{"scripts/wait", "3000"}, tokens)
}

func TestTokenizeHook_AbsentParam_ExpandsEmpty(t *testing.T) {
	tokens := TokenizeHook("tool {missing}", nil)
	assert.Equal(t, []string{"tool", ""}, tokens)
}

// --- RunHooks / RunAfterHooks integration ---

// failEntry builds a HookEntry that calls a non-existent binary so the
// hook fails to start. A start failure looks the same natively and under
// js/wasm, so the tests that use it need no real process.
func failEntry(name string) HookEntry {
	return HookEntry{
		Tokens: []string{"/no/such/fail-hook"},
		Name:   name,
	}
}

func TestRunHooks_Empty_ReturnsNil(t *testing.T) {
	var w bytes.Buffer
	result := RunHooks(context.Background(), nil, t.TempDir(), &w)
	assert.Nil(t, result)
}

func TestRunHooks_SingleFail_ReturnsResult(t *testing.T) {
	var w bytes.Buffer
	result := RunHooks(context.Background(), []HookEntry{failEntry("bad")}, t.TempDir(), &w)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ExitCode)
	assert.Contains(t, w.String(), "bad: FAIL")
}

func TestRunHooks_StopsOnFirstFailure(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "second-ran")
	hooks := []HookEntry{
		failEntry("first"),
		{Tokens: []string{"touch", sentinel}, Name: "second"},
	}
	var w bytes.Buffer
	result := RunHooks(context.Background(), hooks, dir, &w)
	require.NotNil(t, result)
	// Under js/wasm no hook can start, so the sentinel would be absent
	// even if the second hook ran. RunHooks logs "running" before it
	// starts a hook, so the log check holds without a real process.
	assert.NotContains(t, w.String(), "hook second: running", "second hook should not have started")
	_, err := os.Stat(sentinel)
	assert.True(t, os.IsNotExist(err), "second hook should not have run after first failed")
}

func TestRunHooks_NameFallsBackToFirstToken(t *testing.T) {
	var w bytes.Buffer
	hook := HookEntry{Tokens: []string{"echo", "hello"}} // no Name set
	RunHooks(context.Background(), []HookEntry{hook}, t.TempDir(), &w)
	// The full "running" line, not a bare "echo": under js/wasm the
	// FAIL line's exec error names "echo" even when the fallback is lost.
	assert.Contains(t, w.String(), "hook echo: running")
}

// --- RunAfterHooks ---

func TestRunAfterHooks_Empty_ReturnsNil(t *testing.T) {
	var w bytes.Buffer
	result := RunAfterHooks(context.Background(), nil, t.TempDir(), &w)
	assert.Nil(t, result)
}

func TestRunAfterHooks_ReturnsFirstFailure(t *testing.T) {
	// Distinct missing binaries make the two failures distinguishable
	// by their error text; equal exit codes cannot tell first from last.
	// A missing binary is a start failure on every platform, js/wasm
	// included, so this test needs no process to run.
	hooks := []HookEntry{
		{Tokens: []string{"/no/such/hook-a"}, Name: "a"},
		{Tokens: []string{"/no/such/hook-b"}, Name: "b"},
	}
	var w bytes.Buffer
	result := RunAfterHooks(context.Background(), hooks, t.TempDir(), &w)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ExitCode)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "hook-a", "should return the first failure")
	assert.NotContains(t, result.Err.Error(), "hook-b", "should not return a later failure")
	assert.Contains(t, w.String(), "hook b: running", "after-hooks keep running past a failure")
}

func TestRunHooks_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	var w bytes.Buffer
	// Sleep would block, but ctx is already cancelled so exec should fail.
	hook := HookEntry{Tokens: []string{"sleep", "999"}, Name: "sleeper"}
	result := RunHooks(ctx, []HookEntry{hook}, t.TempDir(), &w)
	require.NotNil(t, result)
	assert.Contains(t, w.String(), "sleeper: FAIL")
	// Any start failure also yields a FAIL line, so check the
	// cancellation branch tagged the error. A cancel is an interrupt,
	// not a timeout.
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "(interrupted)")
	assert.NotContains(t, result.Err.Error(), "timed out")
}

func TestRunHooks_ExpiredDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	var w bytes.Buffer
	hook := HookEntry{Tokens: []string{"sleep", "999"}, Name: "sleeper"}
	result := RunHooks(ctx, []HookEntry{hook}, t.TempDir(), &w)
	require.NotNil(t, result)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "(timed out)")
}

// --- output lines ---

func TestRunAfterHooks_OutputLines_OnFail(t *testing.T) {
	var w bytes.Buffer
	hooks := []HookEntry{failEntry("teardown")}
	RunAfterHooks(context.Background(), hooks, t.TempDir(), &w)
	out := w.String()
	assert.Contains(t, out, "hook teardown: running")
	assert.Contains(t, out, "hook teardown: FAIL")
}

func TestRunHooks_HookEntryEmptyTokens_Skipped(t *testing.T) {
	hooks := []HookEntry{{Tokens: nil, Name: "empty"}}
	result := RunHooks(context.Background(), hooks, t.TempDir(), io.Discard)
	assert.Nil(t, result)
}

// ExitCode on a non-existent binary returns 1 (exec.ExitError path or start error).
func TestRunHook_NonExistentBinary_ReturnsFailure(t *testing.T) {
	result := runHook(context.Background(), []string{"/no/such/binary"}, t.TempDir())
	require.NotNil(t, result)
	assert.Greater(t, result.ExitCode, 0)
	assert.Error(t, result.Err)
}

func TestRunAfterHooks_EmptyTokens_Skipped(t *testing.T) {
	hooks := []HookEntry{{Tokens: nil, Name: "empty"}}
	result := RunAfterHooks(context.Background(), hooks, t.TempDir(), io.Discard)
	assert.Nil(t, result)
}
