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

// notePgPath returns the control file whose "kill" write posts a kill
// note to the whole note group led by pid. It is a var so a test can
// point it at a path that fails, or at a plain file.
var notePgPath = func(pid int) string {
	return "/proc/" + strconv.Itoa(pid) + "/notepg"
}

// procRoot is the process file system the forced sweep walks. It is a
// var so a test can point it at a fake tree.
var procRoot = "/proc"

// noteGroup is what afterStart captured while the leader was alive: the
// open notepg file and the group's noteid ("" if unreadable).
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

// afterStart opens the recipe's notepg file and reads its noteid while
// the leader is still alive, and keeps both for killGroup. It returns a
// cleanup that closes the file, or nil when the open failed (a leader
// that already exited, say); killGroup then kills only the leader.
func afterStart(cmd *exec.Cmd) func() {
	if cmd.Process == nil {
		return nil
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	f, err := os.OpenFile(notePgPath(cmd.Process.Pid), os.O_WRONLY, 0)
	if err != nil {
		return nil
	}
	g := noteGroup{pg: f, id: readNoteID(filepath.Join(procRoot, pid))}
	notePgsMu.Lock()
	notePgs[cmd] = g
	notePgsMu.Unlock()
	return func() {
		notePgsMu.Lock()
		delete(notePgs, cmd)
		notePgsMu.Unlock()
		_ = f.Close()
	}
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
// ctl kill, unlike a "kill" note, cannot be caught. It opens ctl before
// it reads noteid: the open file is bound to that process, so the write
// fails rather than hit a process that reused the pid in between. One
// pass only: a note-catching member that forks during the sweep can
// leave a child behind. An empty id matches nothing.
func forceKillNoteGroup(id string) int {
	if id == "" {
		return 0
	}
	ents, err := os.ReadDir(procRoot)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if killIfInGroup(filepath.Join(procRoot, e.Name()), id) {
			n++
		}
	}
	return n
}

// killIfInGroup writes "kill" to dir/ctl when dir/noteid is id, and
// reports whether it did.
func killIfInGroup(dir, id string) bool {
	ctl, err := os.OpenFile(filepath.Join(dir, "ctl"), os.O_WRONLY, 0)
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
// period. It first writes "kill" to the notepg file afterStart holds,
// which reaches every member at once but is a note a handler can catch
// (rc with a sigkill function, say). It then sweeps /proc and writes a
// forced "kill" to the ctl file of every process still in the group.
// If afterStart holds no group, or neither step reached a process, only
// the leader is killed, with a catchable note. A nil Process (the
// command never started) is a no-op.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	notePgsMu.Lock()
	g, held := notePgs[cmd]
	notePgsMu.Unlock()
	if held {
		_, werr := g.pg.WriteString("kill")
		if forceKillNoteGroup(g.id) > 0 || werr == nil {
			return
		}
	}
	_ = cmd.Process.Kill()
}
