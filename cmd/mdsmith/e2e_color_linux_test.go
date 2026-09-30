//go:build linux

package main_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openPTY allocates a pseudo-terminal and returns its controlling and
// terminal ends. The test is skipped where no pty is available.
func openPTY(t *testing.T) (ptmx, tty *os.File) {
	t.Helper()
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })
	var unlock int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(),
		syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock)))
	require.Zero(t, errno, "unlock pty")
	var n uint32
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(),
		syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	require.Zero(t, errno, "pty number")
	tty, err = os.OpenFile("/dev/pts/"+strconv.Itoa(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	require.NoError(t, err)
	return ptmx, tty
}

// runOnTerminal runs the binary in dir with stderr on a fresh
// pseudo-terminal and returns what reached that terminal. extraEnv
// entries are appended to the environment.
func runOnTerminal(t *testing.T, dir string, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	ptmx, tty := openPTY(t)
	var got bytes.Buffer
	done := make(chan struct{})
	go func() {
		// Ends with EIO once the last terminal end closes.
		_, _ = io.Copy(&got, ptmx)
		close(done)
	}()

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(envWithCoverDir(coverDir), extraEnv...)
	cmd.Stderr = tty
	err := cmd.Run()
	require.NoError(t, tty.Close())
	<-done
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else {
		require.NoError(t, err)
	}
	return got.String(), code
}

// TestCheckOutput_E2EColorOnTerminal pins the terminal side of the
// color rule through the real binary: text on a terminal is colored;
// --no-color, --color=never, or a non-empty NO_COLOR turns that off;
// and an explicit --color=auto ignores NO_COLOR.
func TestCheckOutput_E2EColorOnTerminal(t *testing.T) {
	dir := outputWorkspace(t)
	for _, tc := range []struct {
		name string
		env  []string
		args []string
		want bool
	}{
		{name: "terminal", want: true},
		{name: "--no-color", args: []string{"--no-color"}},
		{name: "--color=never", args: []string{"--color=never"}},
		{name: "NO_COLOR", env: []string{"NO_COLOR=1"}},
		{name: "FORCE_COLOR=0", env: []string{"FORCE_COLOR=0"}, want: true},
		{
			name: "--color=auto ignores NO_COLOR", want: true,
			env: []string{"NO_COLOR=1"}, args: []string{"--color=auto"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"check"}, tc.args...)
			out, code := runOnTerminal(t, dir, tc.env, append(args, "long.md")...)
			assert.Equal(t, 1, code)
			assert.Contains(t, out, "MDS001")
			assert.Equal(t, tc.want, bytes.Contains([]byte(out), []byte("\033[")), "terminal=%q", out)
		})
	}
}
