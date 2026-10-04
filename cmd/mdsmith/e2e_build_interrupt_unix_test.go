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

// waitPIDFile polls path for up to 10 s until it holds a pid and
// returns it, or 0 when none was recorded in time.
func waitPIDFile(path string) int {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0
}

// startBuildWithChild starts `mdsmith fix --build-only` in a fresh build
// repo whose one recipe runs prelude, then spawns a long-lived child
// that records its pid, then sleeps. It returns the running CLI, its
// stderr, and the child's pid once recorded. On a failed test it reaps
// the CLI and the child, since a failure may mean the kill regressed,
// so neither leaves a sleep 120 behind.
func startBuildWithChild(t *testing.T, prelude string) (*exec.Cmd, *bytes.Buffer, int) {
	t.Helper()
	dir := writeBuildRepo(t, "")
	pidFile := filepath.Join(dir, "child.pid")
	script := "#!/bin/sh\n" + prelude +
		"sleep 120 & echo $! > \"" + pidFile + "\"\nsleep 120\ntouch \"$1\"\n"
	scriptPath := filepath.Join(dir, "spawn.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	reconfigureRecipe(t, dir, "    spawn:\n      command: "+scriptPath+" {outputs}\n")
	writeFixture(t, dir, "doc.md", buildDirective("spawn", "", "out.txt"))

	cmd := exec.Command(binaryPath, "fix", "--no-color", "--build-only", "doc.md")
	cmd.Dir = dir
	cmd.Env = envWithCoverDir(coverDir)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	require.NoError(t, cmd.Start())

	childPID := waitPIDFile(pidFile)
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
	return cmd, stderr, childPID
}

// requireDiedOf asserts that err is the exit of a process that ended by
// sig, as a shell needs to see to stop its own script.
func requireDiedOf(t *testing.T, err error, sig syscall.Signal, stderr string) {
	t.Helper()
	var ee *exec.ExitError
	require.True(t, errors.As(err, &ee), "expected death by %v, got %v", sig, err)
	ws, ok := ee.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.True(t, ws.Signaled(), "expected death by %v, got %v\n%s", sig, err, stderr)
	assert.Equal(t, sig, ws.Signal())
}

// TestE2E_Build_SignalKillsRecipeTree starts `mdsmith fix` on a recipe
// that spawns a long-lived child, sends the CLI the signal once the
// child is running, and asserts the CLI reports an interrupt (not a
// timeout), leaves no recipe process behind, and then ends by the same
// signal.
func TestE2E_Build_SignalKillsRecipeTree(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			cmd, stderr, childPID := startBuildWithChild(t, "")
			require.NoError(t, cmd.Process.Signal(sig))

			err := cmd.Wait()
			requireDiedOf(t, err, sig, stderr.String())
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
	// An unbreakable 300-char line is an MDS001 finding fix cannot
	// repair, so each file prints ~0.5 KB of diagnostics: far more than
	// a pipe buffer holds in total.
	body := "# Title\n\n" + strings.Repeat("x", 300) + "\n"
	for i := range 500 {
		writeFixture(t, dir, fmt.Sprintf("f%d.md", i), body)
	}

	// Nobody drains the output pipe, so fix blocks writing its
	// diagnostics and cannot exit before the signal lands: a signal to
	// an exited but unreaped child would succeed and hide the race.
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pr.Close() })
	cmd := exec.Command(binaryPath, "fix", "--no-color", "--no-build", ".")
	cmd.Dir = dir
	cmd.Env = envWithCoverDir(coverDir)
	cmd.Stdout = pw
	cmd.Stderr = pw
	require.NoError(t, cmd.Start())
	require.NoError(t, pw.Close())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	err = cmd.Wait()
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
	// trap '' TERM makes the recipe tree ignore SIGTERM; only SIGKILL ends it.
	cmd, stderr, childPID := startBuildWithChild(t, "trap '' TERM\n")

	start := time.Now()
	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))
	// Well past escalateAfter (250 ms): a repeat inside it counts as a
	// copy of the first interrupt and would not skip the grace.
	time.Sleep(750 * time.Millisecond)
	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))

	err := cmd.Wait()
	elapsed := time.Since(start)
	requireDiedOf(t, err, syscall.SIGINT, stderr.String())
	assert.Less(t, elapsed, 4*time.Second, "second SIGINT must skip the 5 s SIGTERM grace")
	assert.Contains(t, stderr.String(), "INTERRUPTED")
	assert.Eventually(t, func() bool { return !unixProcessAlive(childPID) },
		6*time.Second, 100*time.Millisecond, "recipe child must not be orphaned")
}

// TestE2E_Build_BrokenStderrAfterInterruptStillReaps covers
// `mdsmith fix --build-jobs 2 2>&1 | tee log` and a Ctrl-C that also
// ends tee. One worker's recipe dies at once and its INTERRUPTED report
// hits the broken pipe; the other recipe ignores SIGTERM and waits out
// the grace. SIGPIPE must not end mdsmith before it SIGKILLs that group,
// or the group is orphaned.
func TestE2E_Build_BrokenStderrAfterInterruptStillReaps(t *testing.T) {
	dir := writeBuildRepo(t, "")
	quickPID := filepath.Join(dir, "quick.pid")
	stuckPGID := filepath.Join(dir, "stuck.pgid")
	quick := "#!/bin/sh\necho $$ > \"" + quickPID + "\"\nsleep 120\ntouch \"$1\"\n"
	stuck := "#!/bin/sh\ntrap '' TERM\necho $$ > \"" + stuckPGID + "\"\nsleep 120\ntouch \"$1\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "quick.sh"), []byte(quick), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stuck.sh"), []byte(stuck), 0o755))
	reconfigureRecipe(t, dir, "    quick:\n      command: "+filepath.Join(dir, "quick.sh")+" {outputs}\n"+
		"    stuck:\n      command: "+filepath.Join(dir, "stuck.sh")+" {outputs}\n")
	writeFixture(t, dir, "a.md", buildDirective("quick", "", "a.txt"))
	writeFixture(t, dir, "b.md", buildDirective("stuck", "", "b.txt"))

	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	cmd := exec.Command(binaryPath, "fix", "--no-color", "--build-only", "--build-jobs", "2", "a.md", "b.md")
	cmd.Dir = dir
	cmd.Env = envWithCoverDir(coverDir)
	cmd.Stdout = pw
	cmd.Stderr = pw
	require.NoError(t, cmd.Start())
	require.NoError(t, pw.Close())

	pgid := waitPIDFile(stuckPGID)
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		if pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	})
	require.NotZero(t, pgid, "stuck recipe pid should have been recorded")
	require.NotZero(t, waitPIDFile(quickPID), "quick recipe pid should have been recorded")

	require.NoError(t, pr.Close()) // the reader (tee) is gone
	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))
	err = cmd.Wait()
	assert.False(t, unixProcessAlive(pgid), "the SIGTERM-ignoring recipe must be reaped before mdsmith exits")
	// The broken pipe does not change how mdsmith ends: by the interrupt.
	requireDiedOf(t, err, syscall.SIGINT, "")
}
