//go:build plan9

package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// configureProcessGroup starts the recipe in its own note group. rfork
// with RFNOTEG makes the child lead a new group, so a note posted to it
// reaches every process that stays in it. rc's `&` also rforks with
// RFNOTEG, so a job a recipe backgrounds that way leads a group of its
// own and escapes the kill, as a setsid daemon escapes it on Unix.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Rfork: syscall.RFNOTEG}
}

// TimeoutKillAction names, for the timeout report, the kill a timed-out
// recipe gets on this platform. afterStart can capture no group (the
// leader exited first, or joined mdsmith's), and then only the leader
// is killed, so the report says so.
const TimeoutKillAction = "killed note group, or only the leader if none was captured"

// procRoot is the process file system: afterStart opens the leader's
// notepg and noteid files under it, and the forced sweep walks it. A
// "kill" written to <procRoot>/<pid>/notepg posts a kill note to the
// whole note group led by pid. It is a var so a test can point it at a
// fake tree.
var procRoot = "/proc"

// maxSweepPasses bounds how many times forceKillNoteGroup lists
// procRoot, so a member that keeps forking cannot hold killGroup.
const maxSweepPasses = 8

// killMember is killIfInGroup, indirected so a test can model a member
// that forks while the sweep runs.
var killMember = killIfInGroup

// noteKill posts a catchable "kill" note to one process. It is
// (*os.Process).Kill, indirected so a test against a fake /proc never
// sends a note to a real process that has a fake pid.
var noteKill = (*os.Process).Kill

// openProcFile opens a file under procRoot. It is os.OpenFile,
// indirected so a test can change the fake /proc between an open and
// the noteid re-check that follows it.
var openProcFile = os.OpenFile

// noteGroup is what afterStart captured while the leader was alive: the
// group's noteid and, when it could be opened, the notepg file (nil
// otherwise).
type noteGroup struct {
	pg *os.File
	id string
}

// notePgs maps a running command to the note group afterStart captured
// for it. The kernel binds an open notepg to the note group, not to the
// process, so a write still reaches the group after the leader exited
// and its /proc entry is gone, and never reaches a group that later
// reuses the pid. Note ids are not reused either. notePgsMu guards it,
// as jobHandlesMu does on Windows.
var (
	notePgsMu sync.Mutex
	notePgs   = map[*exec.Cmd]noteGroup{}
)

// afterStart reads the recipe's noteid and opens its notepg file while
// the leader is still alive, and keeps both for killGroup. It reads the
// noteid first: a leader that exits before the open still leaves the
// id, which is enough for the forced sweep. It keeps nothing, and
// returns nil, when the id cannot be read (the leader already exited)
// or is mdsmith's own: a recipe that joined its parent's note group
// would otherwise turn the timeout on mdsmith and the user's shell.
// The notepg file is kept only if the leader's noteid is still the
// same once it is open, so the file is bound to the recipe's group.
// The cleanup it returns forgets the group and closes the file. A
// leader that exited before afterStart ran leaves nothing to kill its
// group by; killGroup then kills only the leader.
func afterStart(cmd *exec.Cmd) func() {
	if cmd.Process == nil {
		return nil
	}
	dir := procDir(cmd.Process.Pid)
	id := readNoteID(dir)
	if id == "" || id == readNoteID(procDir(os.Getpid())) {
		return nil
	}
	g := noteGroup{id: id}
	if f, err := openProcFile(filepath.Join(dir, "notepg"), os.O_WRONLY, 0); err == nil {
		if readNoteID(dir) == id {
			g.pg = f
		} else {
			_ = f.Close()
		}
	}
	notePgsMu.Lock()
	notePgs[cmd] = g
	notePgsMu.Unlock()
	return func() {
		notePgsMu.Lock()
		delete(notePgs, cmd)
		notePgsMu.Unlock()
		if g.pg != nil {
			_ = g.pg.Close()
		}
	}
}

// procDir returns the /proc directory of pid under procRoot.
func procDir(pid int) string {
	return filepath.Join(procRoot, strconv.Itoa(pid))
}

// readNoteID returns the trimmed contents of dir/noteid, or "" when it
// cannot be read.
func readNoteID(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "noteid"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// forceKillNoteGroup writes "kill" to the ctl file of every process
// under procRoot whose noteid is id, and returns how many it killed. A
// ctl kill, unlike a "kill" note, cannot be caught. A member that
// catches the notepg note can fork after a pass listed procRoot and
// before that pass killed it, so the sweep lists procRoot again until a
// pass kills no process it had not killed already, at most
// maxSweepPasses times. A child forked by a member still finishing an
// rfork when the last pass lists procRoot can still escape. An empty id
// matches nothing, as killIfInGroup refuses it.
func forceKillNoteGroup(id string) int {
	killed := map[string]bool{}
	for range maxSweepPasses {
		ents, err := os.ReadDir(procRoot)
		if err != nil {
			break
		}
		before := len(killed)
		for _, e := range ents {
			name := e.Name()
			if killed[name] {
				continue
			}
			if _, err := strconv.Atoi(name); err != nil {
				continue
			}
			if killMember(filepath.Join(procRoot, name), id) {
				killed[name] = true
			}
		}
		if len(killed) == before {
			break
		}
	}
	return len(killed)
}

// killIfInGroup writes "kill" to dir/ctl when dir/noteid is id, and
// reports whether it did. An empty id matches nothing. It reads noteid
// before it opens ctl, so a sweep skips a process outside the group
// with one read, and reads it again once ctl is open: the open file is
// bound to that process, so a pid reused between the reads fails the
// check or the write rather than kill a stranger.
func killIfInGroup(dir, id string) bool {
	if id == "" || readNoteID(dir) != id {
		return false
	}
	ctl, err := openProcFile(filepath.Join(dir, "ctl"), os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	defer func() { _ = ctl.Close() }()
	if readNoteID(dir) != id {
		return false
	}
	_, err = ctl.WriteString("kill")
	return err == nil
}

// killGroup kills the recipe's whole note group, so a recipe's
// children that stayed in the group die with it. There is no grace
// period. When afterStart captured the group, it first writes "kill"
// to the held notepg file, which reaches every member at once but is a
// note a handler can catch (rc with a sigkill function, say), then
// sweeps /proc and writes a forced "kill" to the ctl file of every
// process still in the group. Last, and always, it force-kills the
// leader (forceKillLeader), which also covers a leader that left the
// group and one afterStart captured nothing for. A nil Process (the
// command never started) is a no-op: afterStart held no group for it,
// and forceKillLeader skips it.
func killGroup(cmd *exec.Cmd) {
	notePgsMu.Lock()
	g, held := notePgs[cmd]
	notePgsMu.Unlock()
	if held {
		if g.pg != nil {
			_, _ = g.pg.WriteString("kill")
		}
		forceKillNoteGroup(g.id)
	}
	forceKillLeader(cmd)
}

// forceKillLeader writes "kill" to the leader's ctl file. Unlike the
// note (*os.Process).Kill posts, a ctl kill cannot be caught; it takes
// effect when the leader next returns from a system call. When ctl
// cannot be opened or written (the leader exited, or made itself
// private) it falls back to noteKill. Plan 9 hands out pids from a
// counter that only grows, so a leader that already exited does not
// leave its pid to a stranger, the same assumption
// (*os.Process).Kill makes. A nil Process is a no-op.
func forceKillLeader(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	ctl, err := openProcFile(filepath.Join(procDir(cmd.Process.Pid), "ctl"), os.O_WRONLY, 0)
	if err == nil {
		_, err = ctl.WriteString("kill")
		_ = ctl.Close()
		if err == nil {
			return
		}
	}
	_ = noteKill(cmd.Process)
}
