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

// stubEscalateAfter sets escalateAfter for one test.
func stubEscalateAfter(t *testing.T, d time.Duration) {
	t.Helper()
	old := escalateAfter
	escalateAfter = d
	t.Cleanup(func() { escalateAfter = old })
}

func TestWatchInterrupts_FirstCancelsSecondForces(t *testing.T) {
	stubEscalateAfter(t, 0)
	sigs := make(chan os.Signal)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	force := make(chan struct{})
	done := make(chan struct{})
	returned := make(chan struct{})
	var first os.Signal
	go func() {
		defer close(returned)
		first = watchInterrupts(sigs, cancel, force, done)
	}()

	sigs <- syscall.SIGTERM
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
	assert.Equal(t, syscall.SIGTERM, first, "the first signal is the one to re-raise")
}

// TestWatchInterrupts_RepeatWithinWindowDoesNotForce covers one
// interrupt delivered twice: npm run forwards the terminal's SIGINT to
// its child, which also got it from the terminal, and bash resends
// SIGHUP to its jobs as the terminal closes. The copy lands within
// escalateAfter of the first and must not skip the SIGTERM grace.
func TestWatchInterrupts_RepeatWithinWindowDoesNotForce(t *testing.T) {
	stubEscalateAfter(t, time.Hour)
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
	sigs <- os.Interrupt // unbuffered: the watcher has read it
	sigs <- syscall.SIGTERM
	close(done)
	<-returned
	select {
	case <-force:
		t.Fatal("a repeat within escalateAfter must not escalate the kill")
	default:
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
	assert.Equal(t, append([]os.Signal{os.Interrupt, syscall.SIGTERM}, hangupSignals...), interruptSignals())

	// A background job of a non-interactive shell starts with SIGINT
	// ignored: Notify must not un-ignore it.
	stubSignalIgnored(t, os.Interrupt)
	assert.NotContains(t, interruptSignals(), os.Interrupt)
}

func TestDispatchInterruptible_AllSignalsIgnoredRunsPlain(t *testing.T) {
	// With nothing to catch, no handler is installed (Notify with no
	// signal would catch every signal), and the pass's own code stands.
	stubSignalIgnored(t, append([]os.Signal{os.Interrupt, syscall.SIGTERM}, hangupSignals...)...)
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
	assert.Nil(t, watchInterrupts(make(chan os.Signal), cancel, force, done))
	require.NoError(t, ctx.Err(), "no signal, no cancel")
}

// TestWatchInterrupts_SignalQueuedAtDoneIsKept covers a signal Notify
// queued just before dispatch returned and signal.Stop ran: the watcher
// sees it and done together, and must not drop the user's interrupt by
// picking done. The race is a random select, so try it many times.
func TestWatchInterrupts_SignalQueuedAtDoneIsKept(t *testing.T) {
	for i := range 200 {
		sigs := make(chan os.Signal, 1)
		sigs <- os.Interrupt
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		close(done)
		first := watchInterrupts(sigs, cancel, make(chan struct{}), done)
		require.Equal(t, os.Interrupt, first, "iteration %d", i)
		require.ErrorIs(t, ctx.Err(), context.Canceled, "iteration %d", i)
		cancel()
	}
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

// stubRaise records what reraiseInterrupt would raise, for one test.
func stubRaise(t *testing.T) *[]os.Signal {
	t.Helper()
	var raised []os.Signal
	oldRaise, oldPending := raiseSignalFn, pendingInterrupt
	raiseSignalFn = func(s os.Signal) { raised = append(raised, s) }
	t.Cleanup(func() { raiseSignalFn, pendingInterrupt = oldRaise, oldPending })
	return &raised
}

func TestReraiseInterrupt(t *testing.T) {
	raised := stubRaise(t)
	pendingInterrupt = nil
	reraiseInterrupt()
	assert.Empty(t, *raised, "no interrupt, nothing to raise")

	pendingInterrupt = syscall.SIGTERM
	reraiseInterrupt()
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, *raised)
}

func TestDispatchInterruptible_UninterruptedKeepsCode(t *testing.T) {
	// With the handler installed and no signal, the pass's own code
	// stands and nothing is left to re-raise.
	stubSignalIgnored(t)
	stubRaise(t)
	pendingInterrupt = nil
	code := dispatchInterruptible(buildPassOpts{interruptible: true}, func(o buildPassOpts) int {
		assert.NoError(t, o.context().Err())
		return 7
	})
	assert.Equal(t, 7, code)
	assert.Nil(t, pendingInterrupt)
}

// stubNotify records what holdBrokenPipe would Notify, for one test.
func stubNotify(t *testing.T) *[][]os.Signal {
	t.Helper()
	var calls [][]os.Signal
	old := notifySignal
	notifySignal = func(_ chan<- os.Signal, sigs ...os.Signal) { calls = append(calls, sigs) }
	t.Cleanup(func() { notifySignal = old })
	return &calls
}

func TestHoldBrokenPipe_CatchesBrokenPipeSignals(t *testing.T) {
	calls := stubNotify(t)
	old := brokenPipeSignals
	t.Cleanup(func() { brokenPipeSignals = old })

	brokenPipeSignals = []os.Signal{syscall.SIGTERM}
	holdBrokenPipe()
	assert.Equal(t, [][]os.Signal{{syscall.SIGTERM}}, *calls)

	// With nothing to catch, Notify must not run: with no signal it
	// would catch every one.
	*calls = nil
	brokenPipeSignals = nil
	holdBrokenPipe()
	assert.Empty(t, *calls)
}
