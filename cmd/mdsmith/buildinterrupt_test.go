package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchInterrupts_FirstCancelsSecondForces(t *testing.T) {
	sigs := make(chan os.Signal)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	force := make(chan struct{})
	done := make(chan struct{})
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		watchInterrupts(sigs, cancel, force, done)
	}()

	sigs <- os.Interrupt
	<-ctx.Done()
	select {
	case <-force:
		t.Fatal("one signal must not escalate the kill")
	default:
	}

	sigs <- os.Interrupt
	select {
	case <-force:
	case <-time.After(5 * time.Second):
		t.Fatal("second signal must close force")
	}

	// A third signal is swallowed, not a panic on a closed channel.
	sigs <- os.Interrupt
	close(done)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher must return once done closes")
	}
}

// stubSignalIgnored makes signalIgnored report the given signals as
// ignored at startup for one test.
func stubSignalIgnored(t *testing.T, ignored ...os.Signal) {
	t.Helper()
	old := signalIgnored
	signalIgnored = func(s os.Signal) bool {
		for _, ig := range ignored {
			if s == ig {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { signalIgnored = old })
}

func TestInterruptSignals_SkipsIgnored(t *testing.T) {
	stubSignalIgnored(t)
	assert.Equal(t, []os.Signal{os.Interrupt, syscall.SIGTERM}, interruptSignals())

	// A background job of a non-interactive shell starts with SIGINT
	// ignored: Notify must not un-ignore it.
	stubSignalIgnored(t, os.Interrupt)
	assert.NotContains(t, interruptSignals(), os.Interrupt)
}

func TestDispatchInterruptible_AllSignalsIgnoredRunsPlain(t *testing.T) {
	// With nothing to catch, no handler is installed (Notify with no
	// signal would catch every signal), and the pass's own code stands.
	stubSignalIgnored(t, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	code := dispatchInterruptible(buildPassOpts{ctx: ctx, interruptible: true}, func(o buildPassOpts) int {
		calls++
		assert.Equal(t, ctx, o.ctx, "dispatch gets the caller's context unchanged")
		return 0
	})
	assert.Equal(t, 1, calls)
	assert.Equal(t, 0, code)
}

func TestDispatchInterruptible_NotInterruptibleRunsPlain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code := dispatchInterruptible(buildPassOpts{ctx: ctx}, func(o buildPassOpts) int {
		assert.Equal(t, ctx, o.ctx)
		return 0
	})
	assert.Equal(t, 0, code)
}

func TestWatchInterrupts_DoneBeforeAnySignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	force := make(chan struct{})
	done := make(chan struct{})
	close(done)
	watchInterrupts(make(chan os.Signal), cancel, force, done)
	require.NoError(t, ctx.Err(), "no signal, no cancel")
}

func TestWatchInterrupts_DoneAfterOneSignal(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	force := make(chan struct{})
	done := make(chan struct{})
	sigs <- os.Interrupt
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		watchInterrupts(sigs, cancel, force, done)
	}()
	<-ctx.Done()
	close(done)
	<-returned
	select {
	case <-force:
		t.Fatal("force must stay open after one signal")
	default:
	}
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}
