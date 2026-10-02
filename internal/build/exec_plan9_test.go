//go:build plan9

package build

import (
	"bytes"
	"context"
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

// writeRC writes an executable rc script; plan9 has no /bin/sh.
func writeRC(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/rc\n"+body+"\n"), 0o755))
	return p
}

// rcQuote quotes s as one rc word.
func rcQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// spawnInGroup returns rc lines that background `sleep 120`, move it
// back into the recipe's note group (rc's & gives it a group of its
// own), and record its pid in pidFile.
func spawnInGroup(pidFile string) string {
	return "ng=`{cat /proc/$pid/noteid}\n" +
		"sleep 120 &\n" +
		"echo $ng > /proc/$apid/noteid\n" +
		"echo $apid > " + rcQuote(pidFile)
}

// procAlive reports whether /proc/<pid> still exists.
func procAlive(pid int) bool {
	_, err := os.Stat("/proc/" + strconv.Itoa(pid))
	return err == nil
}

// readPID waits for pidFile to hold a decimal pid and returns it.
func readPID(t *testing.T, pidFile string) int {
	t.Helper()
	var pid int
	require.Eventually(t, func() bool {
		b, rerr := os.ReadFile(pidFile)
		if rerr != nil {
			return false
		}
		n, perr := strconv.Atoi(strings.TrimSpace(string(b)))
		pid = n
		return perr == nil
	}, 6*time.Second, 50*time.Millisecond, "child pid should be recorded")
	return pid
}

// stubNotePgPath points notePgPath at path for one test.
func stubNotePgPath(t *testing.T, path string) {
	t.Helper()
	old := notePgPath
	notePgPath = func(int) string { return path }
	t.Cleanup(func() { notePgPath = old })
}

func TestConfigureProcessGroup_Plan9_SetsRFNOTEG(t *testing.T) {
	cmd := &exec.Cmd{}
	configureProcessGroup(cmd)
	require.NotNil(t, cmd.SysProcAttr)
	assert.NotZero(t, cmd.SysProcAttr.Rfork&syscall.RFNOTEG)
}

func TestAfterStart_Plan9_NilProcess(t *testing.T) {
	assert.Nil(t, afterStart(&exec.Cmd{}))
}

func TestAfterStart_Plan9_OpenFailsReturnsNil(t *testing.T) {
	stubProcRoot(t)
	stubNotePgPath(t, "/no/such/notepg")
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.Nil(t, afterStart(cmd))
	notePgsMu.Lock()
	_, held := notePgs[cmd]
	notePgsMu.Unlock()
	assert.False(t, held)
}

func TestKillGroup_Plan9_WritesKillToHeldFile(t *testing.T) {
	// killGroup must write to the file afterStart opened, not reopen
	// /proc/<pid>/notepg, which is gone once the leader has exited.
	// An empty fake /proc keeps the sweep off real pid 42.
	stubProcRoot(t)
	path := filepath.Join(t.TempDir(), "notepg")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	stubNotePgPath(t, path)

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	cleanup := afterStart(cmd)
	require.NotNil(t, cleanup)
	killGroup(cmd)
	cleanup()

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "kill", string(b))
	notePgsMu.Lock()
	_, held := notePgs[cmd]
	notePgsMu.Unlock()
	assert.False(t, held, "cleanup must forget the file")
}

// fakeProc builds a fake /proc entry with the given noteid and an
// empty ctl file, and returns the ctl path.
func fakeProc(t *testing.T, root, pid, noteid string) string {
	t.Helper()
	dir := filepath.Join(root, pid)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noteid"), []byte(noteid), 0o600))
	ctl := filepath.Join(dir, "ctl")
	require.NoError(t, os.WriteFile(ctl, nil, 0o600))
	return ctl
}

// stubProcRoot points procRoot at a temp dir for one test.
func stubProcRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	old := procRoot
	procRoot = root
	t.Cleanup(func() { procRoot = old })
	return root
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

func TestForceKillNoteGroup_Plan9_KillsOnlyMatchingNoteID(t *testing.T) {
	root := stubProcRoot(t)
	member := fakeProc(t, root, "100", "7")
	padded := fakeProc(t, root, "102", "      7 ")
	other := fakeProc(t, root, "101", "8")
	// A non-numeric entry and an entry with no noteid are skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "trace"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "103"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "103", "ctl"), nil, 0o600))

	assert.Equal(t, 2, forceKillNoteGroup("7"))
	assert.Equal(t, "kill", readFile(t, member))
	assert.Equal(t, "kill", readFile(t, padded))
	assert.Empty(t, readFile(t, other))
	assert.Empty(t, readFile(t, filepath.Join(root, "103", "ctl")))
}

func TestForceKillNoteGroup_Plan9_UnreadableRootKillsNone(t *testing.T) {
	old := procRoot
	procRoot = "/no/such/proc"
	t.Cleanup(func() { procRoot = old })
	assert.Zero(t, forceKillNoteGroup("7"))
}

func TestForceKillNoteGroup_Plan9_EmptyIDKillsNone(t *testing.T) {
	// afterStart records "" when it could not read the noteid. An entry
	// whose noteid is also unreadable must not match it.
	root := stubProcRoot(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "100"), 0o755))
	ctl := filepath.Join(root, "100", "ctl")
	require.NoError(t, os.WriteFile(ctl, nil, 0o600))
	assert.Zero(t, forceKillNoteGroup(""))
	assert.Empty(t, readFile(t, ctl))
}

func TestForceKillNoteGroup_Plan9_NoCtlSkipped(t *testing.T) {
	// An entry whose ctl cannot be opened (the process exited during
	// the sweep) is skipped, not counted.
	root := stubProcRoot(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "100"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "100", "noteid"), []byte("7"), 0o600))
	assert.Zero(t, forceKillNoteGroup("7"))
}

func TestAfterStart_Plan9_RecordsNoteID(t *testing.T) {
	// killGroup's forced sweep needs the noteid afterStart read while
	// the leader was alive.
	root := stubProcRoot(t)
	ctl := fakeProc(t, root, "42", "9")
	path := filepath.Join(t.TempDir(), "notepg")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	stubNotePgPath(t, path)

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	cleanup := afterStart(cmd)
	require.NotNil(t, cleanup)
	defer cleanup()
	killGroup(cmd)
	assert.Equal(t, "kill", readFile(t, path))
	assert.Equal(t, "kill", readFile(t, ctl), "the group member must get a forced ctl kill")
}

func TestKillGroup_Plan9_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { killGroup(&exec.Cmd{}) })
}

func TestKillGroup_Plan9_FallsBackToLeaderKill(t *testing.T) {
	// When afterStart could not open the notepg file, killGroup must
	// still kill the leader. exec leaves no rc parent behind to leak.
	stubNotePgPath(t, "/no/such/notepg")

	script := writeRC(t, t.TempDir(), "slow.rc", `exec sleep 120`)
	cmd := exec.Command(script)
	configureProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	require.Nil(t, afterStart(cmd))

	killGroup(cmd)
	assert.Eventually(t, func() bool { return !procAlive(pid) },
		6*time.Second, 100*time.Millisecond, "leader should be killed")
}

func TestKillGroup_Plan9_FailedWriteFallsBackToLeaderKill(t *testing.T) {
	// The held notepg write fails and the sweep finds no member (an
	// empty fake /proc), so killGroup must still kill the leader.
	stubProcRoot(t)
	path := filepath.Join(t.TempDir(), "notepg")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	stubNotePgPath(t, path)

	script := writeRC(t, t.TempDir(), "slow.rc", `exec sleep 120`)
	cmd := exec.Command(script)
	configureProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	cleanup := afterStart(cmd)
	require.NotNil(t, cleanup)
	defer cleanup()
	notePgsMu.Lock()
	_ = notePgs[cmd].pg.Close() // make the write fail
	notePgsMu.Unlock()

	killGroup(cmd)
	assert.Eventually(t, func() bool { return !procAlive(pid) },
		6*time.Second, 100*time.Millisecond, "leader should be killed")
}

func TestRunRecipe_Plan9_TimeoutKillsNoteGroup(t *testing.T) {
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "child.pid")
	// The parent backgrounds a long-lived child in its note group,
	// records its pid, then sleeps. On timeout the whole note group
	// must die, including the child.
	script := writeRC(t, t.TempDir(), "spawn.rc", spawnInGroup(pidFile)+"\nsleep 120")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut)

	childPID := readPID(t, pidFile)
	assert.Eventually(t, func() bool { return !procAlive(childPID) },
		6*time.Second, 100*time.Millisecond, "spawned child should not be orphaned")
}

func TestRunRecipe_Plan9_LeaderExitedChildHoldsPipeTimesOut(t *testing.T) {
	// The leader exits at once, but a child it left in its note group
	// keeps the captured stdout pipe open, and /proc/<leader> is gone
	// by the deadline. The kill must still reach the child through the
	// notepg file afterStart opened while the leader was alive.
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "child.pid")
	script := writeRC(t, t.TempDir(), "orphan.rc", spawnInGroup(pidFile)+"\necho started")

	out := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		defExec: defaultExecConfig(),
		stdout:  out,
	})
	require.ErrorContains(t, err, "recipe timed out")
	assert.True(t, timedOut)
	assert.Contains(t, out.String(), "started")

	childPID := readPID(t, pidFile)
	assert.Eventually(t, func() bool { return !procAlive(childPID) },
		6*time.Second, 100*time.Millisecond, "the group kill must reach the child")
}

func TestRunRecipe_Plan9_TimeoutKillsNoteCatchingLeader(t *testing.T) {
	// The leader rc catches the "kill" note (fn sigkill), so the
	// notepg write alone would leave it looping. The forced ctl kill
	// must still end it.
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "leader.pid")
	script := writeRC(t, t.TempDir(), "stubborn.rc",
		"fn sigkill {}\necho $pid > "+rcQuote(pidFile)+"\nwhile(){ sleep 120 }")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, time.Since(start), reapWait, "the leader must die on the group kill, not the fallback")

	leader := readPID(t, pidFile)
	assert.Eventually(t, func() bool { return !procAlive(leader) },
		6*time.Second, 100*time.Millisecond, "a note-catching leader must still die")
}
