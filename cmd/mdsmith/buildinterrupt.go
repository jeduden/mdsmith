package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	buildexec "github.com/jeduden/mdsmith/internal/build"
	"github.com/jeduden/mdsmith/internal/config"
)

// runBuildPassInterruptible runs the build pass with SIGINT, SIGTERM,
// and on Unix SIGHUP (interruptSignals) cancelling its context while it
// dispatches recipes and hooks, so each
// running recipe's process group is killed before mdsmith exits; recipes
// run in their own group and would otherwise survive the terminal's
// interrupt. A second signal, at least escalateAfter after the first,
// escalates: the Unix kill skips the rest of
// its SIGTERM grace and sends SIGKILL (buildexec.WithForceKill), so
// mdsmith still reaps every recipe but does not make an impatient user
// wait. The handler covers only the dispatch of a pass that can start a
// recipe or hook (dispatchInterruptible): the target scan, the trust
// gate, a pass with no target, the lint-fix pass, and the dry-run,
// check-stale, and explain modes keep the default action, so the signal
// ends mdsmith at once. An interrupted dispatch exits 2, whatever a hook
// it cut short returned.
func runBuildPassInterruptible(
	cfg *config.Config, cfgPath string, files []string, opts buildPassOpts, w io.Writer,
) int {
	opts.interruptible = true
	return runBuildPass(cfg, cfgPath, files, opts, w)
}

// dispatchInterruptible runs dispatch, the step of the build pass that
// starts recipes and hooks. When opts.interruptible is set, the pass can
// start a process, and some interrupt signal is not ignored, the signals
// interruptSignals returns cancel the context dispatch receives in its
// opts (see runBuildPassInterruptible), and an interrupted dispatch
// returns 2. Otherwise dispatch runs with opts unchanged.
func dispatchInterruptible(opts buildPassOpts, dispatch func(buildPassOpts) int) int {
	if !opts.interruptible || !opts.runsProcesses() {
		return dispatch(opts)
	}
	watch := interruptSignals()
	if len(watch) == 0 {
		// Every interrupt signal is ignored: there is nothing to catch,
		// and Notify with no signal would catch them all.
		return dispatch(opts)
	}
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, watch...)

	force := make(chan struct{})
	ctx, cancel := context.WithCancel(buildexec.WithForceKill(opts.context(), force))
	defer cancel()
	done := make(chan struct{})
	watched := make(chan struct{})
	var first os.Signal
	go func() {
		defer close(watched)
		first = watchInterrupts(sigs, func() { holdBrokenPipe(); cancel() }, force, done)
	}()
	// Stop before the watcher ends, so a signal that lands after dispatch
	// returned takes its default action instead of sitting unread in sigs.
	stop := func() { signal.Stop(sigs); close(done); <-watched }

	opts.ctx = ctx
	code := dispatch(opts)
	stop()
	if first != nil && pendingInterrupt == nil {
		pendingInterrupt = first
	}
	// Read ctx before the deferred cancel, which ends it.
	if ctx.Err() != nil {
		return 2
	}
	return code
}

// notifySignal is signal.Notify, a var so a test can check
// holdBrokenPipe without changing the test binary's signal handling.
var notifySignal = signal.Notify

// holdBrokenPipe catches brokenPipeSignals (SIGPIPE on Unix) for the
// rest of the process once an interrupt arrives. A Ctrl-C often also
// ends the reader of mdsmith's output (`mdsmith fix 2>&1 | tee log`),
// and Go dies of SIGPIPE on a write to a broken stdout or stderr: one
// worker's INTERRUPTED report would then end mdsmith while another
// waits out its grace, orphaning that recipe group. Caught, the write
// just fails and mdsmith reaps every group, then dies of the interrupt.
// The hold is never released: the run is ending. With no signal to
// catch it does nothing, since Notify with none catches them all.
func holdBrokenPipe() {
	if len(brokenPipeSignals) == 0 {
		return
	}
	notifySignal(make(chan os.Signal, 1), brokenPipeSignals...)
}

// pendingInterrupt is the first signal an interruptible dispatch
// caught, or nil. main re-raises it (reraiseInterrupt) once the run's
// output is written, so mdsmith ends by that signal like any
// interrupted process: a calling shell script then stops too (bash's
// wait-and-cooperative-exit rule) instead of reading exit 2 as a plain
// failure it may ignore.
var pendingInterrupt os.Signal

// raiseSignalFn indirects raiseSignal so a test can observe the raise
// without ending the test binary.
var raiseSignalFn = raiseSignal

// reraiseInterrupt re-raises pendingInterrupt with its default action.
// Where the platform cannot (raiseSignal is a no-op, or the signal does
// not end the process), it returns and main exits with the run's code,
// 2 for an interrupted dispatch.
func reraiseInterrupt() {
	if pendingInterrupt != nil {
		raiseSignalFn(pendingInterrupt)
	}
}

// signalIgnored is signal.Ignored, indirected so a test can model a
// process started with an interrupt signal ignored.
var signalIgnored = signal.Ignored

// interruptSignals returns the signals that cancel the build: SIGINT,
// SIGTERM, and on Unix SIGHUP (hangupSignals: a closed terminal or a
// dropped SSH session), minus any the process started with ignored. A
// background job of a non-interactive shell starts with SIGINT ignored,
// and nohup ignores SIGHUP; Notify would un-ignore them, so a Ctrl-C
// meant for the foreground job would cancel this build.
func interruptSignals() []os.Signal {
	candidates := append([]os.Signal{os.Interrupt, syscall.SIGTERM}, hangupSignals...)
	out := make([]os.Signal, 0, len(candidates))
	for _, s := range candidates {
		if !signalIgnored(s) {
			out = append(out, s)
		}
	}
	return out
}

// escalateAfter is how long after the first interrupt a further one
// must arrive to escalate to SIGKILL. One interrupt is often delivered
// twice within milliseconds: npm run forwards the terminal's SIGINT to
// a child that already got it, and bash resends SIGHUP to its jobs as
// the terminal closes. A copy inside the window is that same interrupt,
// not an impatient user, so it keeps the SIGTERM grace. A var so a test
// can set it.
var escalateAfter = 250 * time.Millisecond

// watchInterrupts turns signals into the build's two kill stages: the
// first one received on sigs calls cancel (recipes get SIGTERM and the
// grace period), the second closes force (SIGKILL at once) unless it
// arrives within escalateAfter of the first, which marks it a copy of
// the first and swallows it. Later signals are swallowed, because exiting before every recipe group is
// reaped would orphan it. It returns, when done is closed, the first
// signal received, or nil. A signal Notify queued in sigs before
// signal.Stop still counts when done closes alongside it: select picks
// a ready case at random, and dropping it would swallow the interrupt.
func watchInterrupts(
	sigs <-chan os.Signal, cancel context.CancelFunc, force chan<- struct{}, done <-chan struct{},
) os.Signal {
	var first os.Signal
	select {
	case first = <-sigs:
		cancel()
	case <-done:
		select {
		case first = <-sigs:
			cancel()
		default:
		}
		return first
	}
	firstAt := time.Now()
	for escalated := false; !escalated; {
		select {
		case <-sigs:
			if time.Since(firstAt) >= escalateAfter {
				close(force)
				escalated = true
			}
		case <-done:
			return first
		}
	}
	for {
		select {
		case <-sigs:
		case <-done:
			return first
		}
	}
}
