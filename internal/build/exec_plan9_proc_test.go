//go:build plan9

package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the plan9 kill path against a fake /proc tree. They
// start no process, so they hold no rc dependency; exec_plan9_test.go
// has the tests that run real rc recipes.

// fakeNotePg creates root/pid/notepg as a plain file, so afterStart can
// open it, and returns its path.
func fakeNotePg(t *testing.T, root, pid string) string {
	t.Helper()
	dir := filepath.Join(root, pid)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "notepg")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	return path
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

// stubNoteKill records the pids noteKill is asked to kill instead of
// posting a note, so a fake pid never reaches a real process.
func stubNoteKill(t *testing.T) *[]int {
	t.Helper()
	var pids []int
	old := noteKill
	noteKill = func(p *os.Process) error {
		pids = append(pids, p.Pid)
		return nil
	}
	t.Cleanup(func() { noteKill = old })
	return &pids
}

// stubKillMember wraps killMember for one test: after each kill the
// real one reports, onKill runs with the killed entry's directory.
func stubKillMember(t *testing.T, onKill func(dir string)) {
	t.Helper()
	old := killMember
	killMember = func(dir, id string) bool {
		ok := old(dir, id)
		if ok {
			onKill(dir)
		}
		return ok
	}
	t.Cleanup(func() { killMember = old })
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

// groupOf returns the note group k captured, or nil when it captured
// none.
func groupOf(k groupKiller) *noteGroup { return k.(*noteKiller).group }

func TestAfterStart_Plan9_NilProcess(t *testing.T) {
	assert.Nil(t, groupOf(afterStart(&exec.Cmd{})))
}

func TestAfterStart_Plan9_NoProcEntryCapturesNoGroup(t *testing.T) {
	// A leader that exited before afterStart ran has no /proc entry.
	stubProcRoot(t) // empty, so <root>/42 has neither noteid nor notepg
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.Nil(t, groupOf(afterStart(cmd)))
}

func TestAfterStart_Plan9_UnreadableNoteIDCapturesNoGroup(t *testing.T) {
	// Without the noteid, afterStart cannot tell the notepg file from
	// mdsmith's own group's, so it must not keep it.
	fakeNotePg(t, stubProcRoot(t), "42")
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.Nil(t, groupOf(afterStart(cmd)))
}

func TestAfterStart_Plan9_RefusesOwnNoteGroup(t *testing.T) {
	// A recipe that joined mdsmith's note group before afterStart ran
	// must not turn the timeout kill on mdsmith and the user's shell.
	// os.Getpid reads #c/pid, which on plan9 is the pid of whichever M
	// (its own process) runs the goroutine, so pin it: the fake self
	// entry and afterStart's lookup must use the same pid.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := stubProcRoot(t)
	self := fakeProc(t, root, strconv.Itoa(os.Getpid()), "9")
	fakeProc(t, root, "42", "9")
	fakeNotePg(t, root, "42")
	noted := stubNoteKill(t)

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := afterStart(cmd)
	assert.Nil(t, groupOf(k))
	k.kill(nil)
	assert.Empty(t, readFile(t, self), "mdsmith itself must not be swept")
	assert.Empty(t, *noted)
}

func TestAfterStart_Plan9_KeepsNoteIDWhenNotePgFails(t *testing.T) {
	// The noteid alone still lets the forced sweep reach the group.
	root := stubProcRoot(t)
	fakeProc(t, root, "42", "9")
	member := fakeProc(t, root, "100", "9")
	stubNoteKill(t)

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := afterStart(cmd)
	require.NotNil(t, groupOf(k))
	defer k.close()
	k.kill(nil)
	assert.Equal(t, "kill", readFile(t, member))
}

func TestAfterStart_Plan9_RecordsNoteID(t *testing.T) {
	// kill's forced sweep needs the noteid afterStart read while
	// the leader was alive.
	root := stubProcRoot(t)
	ctl := fakeProc(t, root, "42", "9")
	path := fakeNotePg(t, root, "42")
	stubNoteKill(t) // pid 42 is fake: no note may reach a real process

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := afterStart(cmd)
	require.NotNil(t, groupOf(k))
	defer k.close()
	k.kill(nil)
	assert.Equal(t, "kill", readFile(t, path))
	assert.Equal(t, "kill", readFile(t, ctl), "the group member must get a forced ctl kill")
}

func TestKill_Plan9_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { afterStart(&exec.Cmd{}).kill(nil) })
}

func TestKill_Plan9_WritesKillToHeldFile(t *testing.T) {
	// kill must write to the file afterStart opened, not reopen
	// /proc/<pid>/notepg, which is gone once the leader has exited.
	root := stubProcRoot(t)
	fakeProc(t, root, "42", "9")
	path := fakeNotePg(t, root, "42")
	stubNoteKill(t)

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := afterStart(cmd)
	require.NotNil(t, groupOf(k))
	k.kill(nil)
	k.close()

	assert.Equal(t, "kill", readFile(t, path))
}

func TestClose_Plan9_ClosesHeldNotePg(t *testing.T) {
	// close must release the notepg file afterStart kept, so a recipe
	// does not leak the fd past runRecipe's return.
	root := stubProcRoot(t)
	fakeProc(t, root, "42", "9")
	fakeNotePg(t, root, "42")
	stubNoteKill(t)

	k := afterStart(&exec.Cmd{Process: &os.Process{Pid: 42}})
	require.NotNil(t, groupOf(k))
	pg := groupOf(k).pg
	require.NotNil(t, pg)
	k.close()
	_, err := pg.WriteString("kill")
	assert.ErrorIs(t, err, os.ErrClosed)
	// close drops the file, so a second close (or a kill after it)
	// never touches the closed fd, as jobKiller.close zeroes its job.
	assert.Nil(t, groupOf(k).pg)
	assert.NotPanics(t, k.close)
}

func TestClose_Plan9_NoGroupIsNoOp(t *testing.T) {
	// runRecipe closes every killer, including one for a leader that
	// exited before afterStart ran, which captured no group at all.
	stubProcRoot(t) // empty, so afterStart reads no noteid
	k := afterStart(&exec.Cmd{Process: &os.Process{Pid: 42}})
	require.Nil(t, groupOf(k))
	assert.NotPanics(t, k.close)
}

func TestKill_Plan9_ForceKillsLeaderThatLeftGroup(t *testing.T) {
	// The notepg write succeeds, but the leader moved to another note
	// group, so neither the note nor the sweep reaches it. kill
	// must still write a forced kill to the leader's ctl file.
	root := stubProcRoot(t)
	ctl := fakeProc(t, root, "42", "9")
	fakeNotePg(t, root, "42")
	noted := stubNoteKill(t)

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := afterStart(cmd)
	require.NotNil(t, groupOf(k))
	defer k.close()
	require.NoError(t, os.WriteFile(filepath.Join(root, "42", "noteid"), []byte("5"), 0o600))

	k.kill(nil)
	assert.Equal(t, "kill", readFile(t, ctl), "the leader must get a forced ctl kill")
	assert.Empty(t, *noted)
}

func TestKill_Plan9_FailedWriteStillKillsLeader(t *testing.T) {
	// The held notepg write fails and the sweep finds no member, so the
	// leader's forced kill is all that is left.
	root := stubProcRoot(t)
	ctl := fakeProc(t, root, "42", "9")
	fakeNotePg(t, root, "42")
	stubNoteKill(t)

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := afterStart(cmd)
	require.NotNil(t, groupOf(k))
	defer k.close()
	_ = groupOf(k).pg.Close() // make the write fail
	require.NoError(t, os.WriteFile(filepath.Join(root, "42", "noteid"), []byte("5"), 0o600))

	k.kill(nil)
	assert.Equal(t, "kill", readFile(t, ctl))
}

func TestForceKillLeader_Plan9_WritesCtl(t *testing.T) {
	ctl := fakeProc(t, stubProcRoot(t), "42", "9")
	noted := stubNoteKill(t)
	forceKillLeader(&exec.Cmd{Process: &os.Process{Pid: 42}})
	assert.Equal(t, "kill", readFile(t, ctl))
	assert.Empty(t, *noted, "a ctl kill needs no note")
}

func TestForceKillLeader_Plan9_NoCtlFallsBackToNote(t *testing.T) {
	stubProcRoot(t) // no <root>/42/ctl
	noted := stubNoteKill(t)
	forceKillLeader(&exec.Cmd{Process: &os.Process{Pid: 42}})
	assert.Equal(t, []int{42}, *noted)
}

func TestForceKillLeader_Plan9_NilProcess(t *testing.T) {
	noted := stubNoteKill(t)
	assert.NotPanics(t, func() { forceKillLeader(&exec.Cmd{}) })
	assert.Empty(t, *noted)
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
	// An entry whose noteid is unreadable must not match an empty id.
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

func TestForceKillNoteGroup_Plan9_KillsMemberForkedDuringSweep(t *testing.T) {
	// A note-catching member can fork after the first pass listed
	// /proc. The sweep must list it again and kill the new child.
	root := stubProcRoot(t)
	fakeProc(t, root, "100", "7")
	var child string
	stubKillMember(t, func(string) {
		if child == "" {
			child = fakeProc(t, root, "200", "7")
		}
	})

	assert.Equal(t, 2, forceKillNoteGroup("7"))
	assert.Equal(t, "kill", readFile(t, child), "the child forked during the sweep must die")
}

func TestForceKillNoteGroup_Plan9_StopsAfterMaxPasses(t *testing.T) {
	// A member that forks on every pass cannot hold the sweep forever.
	root := stubProcRoot(t)
	fakeProc(t, root, "100", "7")
	next := 101
	stubKillMember(t, func(string) {
		fakeProc(t, root, strconv.Itoa(next), "7")
		next++
	})

	assert.Equal(t, maxSweepPasses, forceKillNoteGroup("7"))
}

func TestReadNoteID_Plan9(t *testing.T) {
	root := t.TempDir()
	fakeProc(t, root, "100", "      7 ")
	assert.Equal(t, "7", readNoteID(filepath.Join(root, "100")), "padding is trimmed")
	assert.Empty(t, readNoteID(filepath.Join(root, "101")), "a missing noteid reads as empty")
}

func TestKillIfInGroup_Plan9(t *testing.T) {
	root := t.TempDir()

	member := fakeProc(t, root, "100", "7")
	assert.True(t, killIfInGroup(filepath.Join(root, "100"), "7"))
	assert.Equal(t, "kill", readFile(t, member))

	other := fakeProc(t, root, "101", "8")
	assert.False(t, killIfInGroup(filepath.Join(root, "101"), "7"))
	assert.Empty(t, readFile(t, other), "a process in another group is left alone")

	noCtl := filepath.Join(root, "102")
	require.NoError(t, os.MkdirAll(noCtl, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(noCtl, "noteid"), []byte("7"), 0o600))
	assert.False(t, killIfInGroup(noCtl, "7"), "an exited process cannot be killed")

	noID := filepath.Join(root, "103")
	require.NoError(t, os.MkdirAll(noID, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(noID, "ctl"), nil, 0o600))
	assert.False(t, killIfInGroup(noID, ""), "an empty id matches nothing")
	assert.Empty(t, readFile(t, filepath.Join(noID, "ctl")))
}

func TestTimeoutKillAction_Plan9(t *testing.T) {
	assert.Equal(t, "killed note group, or only the leader if none was captured", TimeoutKillAction)
}

// stubOpenProcFile runs after(name) once openProcFile has opened name,
// so a test can change the fake /proc between an open and the noteid
// re-check that follows it.
func stubOpenProcFile(t *testing.T, after func(name string)) {
	t.Helper()
	old := openProcFile
	openProcFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		f, err := old(name, flag, perm)
		if err == nil {
			after(name)
		}
		return f, err
	}
	t.Cleanup(func() { openProcFile = old })
}

func TestKillIfInGroup_Plan9_NoteIDChangedAfterCtlOpen(t *testing.T) {
	// The pid was reused (or the process changed group) between the
	// first noteid read and the ctl open: the re-check must refuse.
	root := t.TempDir()
	ctl := fakeProc(t, root, "100", "7")
	stubOpenProcFile(t, func(name string) {
		if filepath.Base(name) == "ctl" {
			require.NoError(t, os.WriteFile(filepath.Join(root, "100", "noteid"), []byte("8"), 0o600))
		}
	})
	assert.False(t, killIfInGroup(filepath.Join(root, "100"), "7"))
	assert.Empty(t, readFile(t, ctl), "a process no longer in the group is left alone")
}

func TestAfterStart_Plan9_DropsNotePgWhenNoteIDChangedAfterOpen(t *testing.T) {
	// The leader changed note group after afterStart read its noteid
	// and before the notepg open: the file is bound to the new group,
	// so afterStart must not keep it, but keeps the id it read.
	root := stubProcRoot(t)
	fakeProc(t, root, "42", "9")
	path := fakeNotePg(t, root, "42")
	member := fakeProc(t, root, "100", "9")
	stubNoteKill(t)
	stubOpenProcFile(t, func(name string) {
		if filepath.Base(name) == "notepg" {
			require.NoError(t, os.WriteFile(filepath.Join(root, "42", "noteid"), []byte("10"), 0o600))
		}
	})

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := afterStart(cmd)
	require.NotNil(t, groupOf(k))
	defer k.close()
	k.kill(nil)
	assert.Empty(t, readFile(t, path), "a notepg bound to another group must not get the kill")
	assert.Equal(t, "kill", readFile(t, member), "the sweep still reaches the captured group")
}
