//go:build unix

package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_Build_SignalKillsRecipeTree starts `mdsmith fix` on a recipe
// that spawns a long-lived child, sends the CLI the signal once the
// child is running, and asserts the CLI exits non-zero, reports an
// interrupt (not a timeout), and leaves no recipe process behind.
func TestE2E_Build_SignalKillsRecipeTree(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := writeBuildRepo(t, "")
			pidFile := filepath.Join(dir, "child.pid")
			script := "#!/bin/sh\nsleep 120 & echo $! > \"" + pidFile + "\"\nsleep 120\ntouch \"$1\"\n"
			scriptPath := filepath.Join(dir, "spawn.sh")
			require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
			reconfigureRecipe(t, dir, "    spawn:\n      command: "+scriptPath+" {outputs}\n")
			writeFixture(t, dir, "doc.md", buildDirective("spawn", "", "out.txt"))

			cmd := exec.Command(binaryPath, "fix", "--no-color", "--build-only", "doc.md")
			cmd.Dir = dir
			cmd.Env = envWithCoverDir(coverDir)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Start())

			var childPID int
			deadline := time.Now().Add(10 * time.Second)
			for childPID == 0 && time.Now().Before(deadline) {
				if b, err := os.ReadFile(pidFile); err == nil {
					childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
				}
				time.Sleep(20 * time.Millisecond)
			}
			// A failed test may mean the kill regressed: reap the CLI and
			// the child then, so they do not leave a sleep 120 behind.
			t.Cleanup(func() {
				if !t.Failed() {
					return
				}
				_ = cmd.Process.Kill()
				if childPID > 0 {
					_ = syscall.Kill(childPID, syscall.SIGKILL)
				}
			})
			require.NotZero(t, childPID, "child pid should have been recorded")
			require.NoError(t, cmd.Process.Signal(sig))

			err := cmd.Wait()
			ee, ok := err.(*exec.ExitError)
			require.True(t, ok, "expected non-zero exit, got %v", err)
			assert.Equal(t, 2, ee.ExitCode(), stderr.String())
			assert.Contains(t, stderr.String(), "INTERRUPTED")
			assert.NotContains(t, stderr.String(), "TIMEOUT")
			assert.Eventually(t, func() bool { return !unixProcessAlive(childPID) },
				6*time.Second, 100*time.Millisecond, "recipe child must not be orphaned")
		})
	}
}

// TestE2E_Fix_NoBuildKeepsDefaultSignalAction checks that the build
// pass's interrupt handler does not cover a run that starts no recipe:
// SIGTERM must still end `mdsmith fix --no-build` at once instead of
// being swallowed until the lint-fix pass finishes.
func TestE2E_Fix_NoBuildKeepsDefaultSignalAction(t *testing.T) {
	dir := t.TempDir()
	body := "# Title\n\n" + strings.Repeat("Some text with trailing spaces   \n\n", 40)
	for i := range 1500 {
		writeFixture(t, dir, fmt.Sprintf("f%d.md", i), body)
	}

	cmd := exec.Command(binaryPath, "fix", "--no-color", "--no-build", ".")
	cmd.Dir = dir
	cmd.Env = envWithCoverDir(coverDir)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	time.Sleep(100 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Wait()
		t.Skipf("fix finished before the signal was sent: %v", err)
	}
	err := cmd.Wait()
	var ee *exec.ExitError
	require.True(t, errors.As(err, &ee), "fix must not survive SIGTERM, got %v", err)
	ws, ok := ee.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.True(t, ws.Signaled(), "fix must die of SIGTERM, got %v", err)
}

// TestE2E_Build_SecondSignalSkipsGrace checks that a second Ctrl-C
// escalates the kill: a recipe tree that ignores SIGTERM would hold
// mdsmith for the 5 s grace period, but the second SIGINT sends SIGKILL
// at once. The tree still dies, so nothing is orphaned.
func TestE2E_Build_SecondSignalSkipsGrace(t *testing.T) {
	dir := writeBuildRepo(t, "")
	pidFile := filepath.Join(dir, "child.pid")
	script := "#!/bin/sh\ntrap '' TERM\nsleep 120 & echo $! > \"" + pidFile + "\"\nsleep 120\ntouch \"$1\"\n"
	scriptPath := filepath.Join(dir, "stubborn.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	reconfigureRecipe(t, dir, "    stubborn:\n      command: "+scriptPath+" {outputs}\n")
	writeFixture(t, dir, "doc.md", buildDirective("stubborn", "", "out.txt"))

	cmd := exec.Command(binaryPath, "fix", "--no-color", "--build-only", "doc.md")
	cmd.Dir = dir
	cmd.Env = envWithCoverDir(coverDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())

	var childPID int
	deadline := time.Now().Add(10 * time.Second)
	for childPID == 0 && time.Now().Before(deadline) {
		if b, err := os.ReadFile(pidFile); err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		_ = cmd.Process.Kill()
		if childPID > 0 {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		}
	})
	require.NotZero(t, childPID, "child pid should have been recorded")

	start := time.Now()
	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))

	err := cmd.Wait()
	elapsed := time.Since(start)
	ee, ok := err.(*exec.ExitError)
	require.True(t, ok, "expected non-zero exit, got %v", err)
	assert.Equal(t, 2, ee.ExitCode(), stderr.String())
	assert.Less(t, elapsed, 4*time.Second, "second SIGINT must skip the 5 s SIGTERM grace")
	assert.Contains(t, stderr.String(), "INTERRUPTED")
	assert.Eventually(t, func() bool { return !unixProcessAlive(childPID) },
		6*time.Second, 100*time.Millisecond, "recipe child must not be orphaned")
}
