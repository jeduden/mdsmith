//go:build plan9

package build

import (
	"bytes"
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

// pidIn returns the decimal pid pidFile holds, and whether it held one.
func pidIn(pidFile string) (int, bool) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return n, err == nil
}

// readPID waits for pidFile to hold a decimal pid and returns it.
func readPID(t *testing.T, pidFile string) int {
	t.Helper()
	var pid int
	require.Eventually(t, func() bool {
		n, ok := pidIn(pidFile)
		pid = n
		return ok
	}, 6*time.Second, 50*time.Millisecond, "pid should be recorded")
	return pid
}

// firedAt waits for the deadline and returns when it passed.
func (c *readyDeadline) firedAt() time.Time {
	<-c.done
	return c.at
}

// pidRecorded reports whether pidFile holds a pid yet.
func pidRecorded(pidFile string) func() bool {
	return func() bool {
		_, ok := pidIn(pidFile)
		return ok
	}
}

func TestConfigureProcessGroup_Plan9_SetsRFNOTEG(t *testing.T) {
	cmd := &exec.Cmd{}
	configureProcessGroup(cmd)
	require.NotNil(t, cmd.SysProcAttr)
	assert.NotZero(t, cmd.SysProcAttr.Rfork&syscall.RFNOTEG)
}

func TestKill_Plan9_FallsBackToLeaderKill(t *testing.T) {
	// When afterStart captured no group, kill must still kill the
	// leader. exec leaves no rc parent behind to leak.
	stubProcRoot(t) // empty, so afterStart reads no noteid

	script := writeRC(t, t.TempDir(), "slow.rc", `exec sleep 120`)
	cmd := exec.Command(script)
	configureProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	k := afterStart(cmd)
	require.Nil(t, groupOf(k))

	k.kill(nil)
	assert.Eventually(t, func() bool { return !procAlive(pid) },
		6*time.Second, 100*time.Millisecond, "leader should be killed")
}

func TestKill_Plan9_ForceKillsNoteCatchingLeaderWithoutGroup(t *testing.T) {
	// With no note group held (afterStart failed), kill kills only
	// the leader. Process.Kill posts a "kill" note, which a leader with
	// fn sigkill catches, so kill must use the leader's ctl file.
	pidFile := filepath.Join(t.TempDir(), "leader.pid")
	script := writeRC(t, t.TempDir(), "stubborn.rc",
		"fn sigkill {}\necho $pid > "+rcQuote(pidFile)+"\nwhile(){ sleep 120 }")
	cmd := exec.Command(script)
	configureProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	// Register the cleanup before anything can fail the test: the leader
	// catches the "kill" note, so nothing else would end its loop.
	id := readNoteID(procDir(cmd.Process.Pid))
	t.Cleanup(func() {
		forceKillNoteGroup(id) // the loop's sleep outlives its leader
		forceKillLeader(cmd)
		_ = cmd.Wait()
	})
	pid := readPID(t, pidFile)

	(&noteKiller{cmd: cmd}).kill(nil) // no group held, as when afterStart captured none
	assert.Eventually(t, func() bool { return !procAlive(pid) },
		6*time.Second, 100*time.Millisecond, "a note-catching leader must still die")
}

func TestRunRecipe_Plan9_TimeoutKillsNoteGroup(t *testing.T) {
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "child.pid")
	// The parent backgrounds a long-lived child in its note group,
	// records its pid, then sleeps. On timeout the whole note group
	// must die, including the child. The deadline passes only once the
	// child is back in the group.
	script := writeRC(t, t.TempDir(), "spawn.rc", spawnInGroup(pidFile)+"\nsleep 120")

	_, timedOut, err := runRecipe(deadlineWhen(t, pidRecorded(pidFile)), runOpts{
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
	// The leader exits, but a child it left in its note group keeps the
	// captured stdout pipe open, and /proc/<leader> is gone by the
	// deadline. The kill must still reach the child through what
	// afterStart captured while the leader was alive. The deadline
	// passes only once the leader is gone.
	stage := t.TempDir()
	leaderFile := filepath.Join(stage, "leader.pid")
	pidFile := filepath.Join(stage, "child.pid")
	script := writeRC(t, t.TempDir(), "orphan.rc",
		"echo started\necho $pid > "+rcQuote(leaderFile)+"\n"+spawnInGroup(pidFile))

	out := &bytes.Buffer{}
	ctx := deadlineWhen(t, func() bool {
		leader, ok := pidIn(leaderFile)
		_, child := pidIn(pidFile)
		return ok && child && !procAlive(leader)
	})
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
	// must still end it. The deadline passes once the handler is set.
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "leader.pid")
	// The leader copies its noteid to noteDir/noteid before it records
	// its pid, so readNoteID(noteDir) has it once the deadline passes.
	noteDir := t.TempDir()
	script := writeRC(t, t.TempDir(), "stubborn.rc",
		"fn sigkill {}\ncat /proc/$pid/noteid > "+rcQuote(filepath.Join(noteDir, "noteid"))+
			"\necho $pid > "+rcQuote(pidFile)+"\nwhile(){ sleep 120 }")
	t.Cleanup(func() {
		// The loop can fork one more sleep after the sweep's last pass
		// listed /proc; sweep the group again so it does not outlive
		// the test.
		forceKillNoteGroup(readNoteID(noteDir))
	})

	ctx := deadlineWhen(t, pidRecorded(pidFile))
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, time.Since(ctx.firedAt()), reapWait, "the leader must die on the group kill, not the fallback")

	leader := readPID(t, pidFile)
	assert.Eventually(t, func() bool { return !procAlive(leader) },
		6*time.Second, 100*time.Millisecond, "a note-catching leader must still die")
}
