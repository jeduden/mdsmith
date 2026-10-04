//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterruptSignals_IncludesHangup(t *testing.T) {
	// Closing the terminal or dropping an SSH session sends SIGHUP: it
	// must cancel the build too, or the recipe groups are orphaned.
	stubSignalIgnored(t)
	assert.Contains(t, interruptSignals(), syscall.SIGHUP)

	// Under nohup SIGHUP starts ignored and must stay so.
	stubSignalIgnored(t, syscall.SIGHUP)
	assert.NotContains(t, interruptSignals(), syscall.SIGHUP)
}

func TestRaiseSignal_ResetsAndSignalsSelf(t *testing.T) {
	var sent []syscall.Signal
	oldKill, oldWait := killSelf, raiseWait
	killSelf = func(s syscall.Signal) error { sent = append(sent, s); return nil }
	raiseWait = 0
	t.Cleanup(func() { killSelf, raiseWait = oldKill, oldWait })

	raiseSignal(syscall.SIGHUP)
	assert.Equal(t, []syscall.Signal{syscall.SIGHUP}, sent)
	// The handler is back to the default action: a caught SIGHUP would
	// otherwise keep the process alive after the raise.
	assert.False(t, signal.Ignored(syscall.SIGHUP))
}

func TestRaiseSignal_NonSyscallSignalIsNoop(t *testing.T) {
	oldKill := killSelf
	killSelf = func(syscall.Signal) error { t.Error("must not signal"); return nil }
	t.Cleanup(func() { killSelf = oldKill })
	raiseSignal(fakeSignal{})
}

// fakeSignal is an os.Signal that is not a syscall.Signal.
type fakeSignal struct{}

func (fakeSignal) String() string { return "fake" }
func (fakeSignal) Signal()        {}

func TestKillSelf_ReachesThisProcess(t *testing.T) {
	// Signal 0 probes delivery without sending anything.
	require.NoError(t, killSelf(0))
}

// TestDispatchInterruptible_RecordsFirstSignal delivers a real SIGHUP
// while dispatch runs: the handler catches it, cancels the dispatch,
// and records it so main re-raises it once output is written.
func TestDispatchInterruptible_RecordsFirstSignal(t *testing.T) {
	stubRaise(t)
	pendingInterrupt = nil
	code := dispatchInterruptible(buildPassOpts{interruptible: true}, func(o buildPassOpts) int {
		require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGHUP))
		select {
		case <-o.context().Done():
		case <-time.After(10 * time.Second):
			t.Error("SIGHUP must cancel the dispatch")
		}
		return 0
	})
	assert.Equal(t, 2, code)
	assert.Equal(t, syscall.SIGHUP, pendingInterrupt)
}
