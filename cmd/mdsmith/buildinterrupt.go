package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	buildexec "github.com/jeduden/mdsmith/internal/build"
	"github.com/jeduden/mdsmith/internal/config"
)

// runBuildPassInterruptible runs the build pass with SIGINT and SIGTERM
// cancelling its context while it dispatches recipes and hooks, so each
// running recipe's process group is killed before mdsmith exits; recipes
// run in their own group and would otherwise survive the terminal's
// interrupt. A second signal escalates: the Unix kill skips the rest of
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
	go func() {
		defer close(watched)
		watchInterrupts(sigs, cancel, force, done)
	}()
	// Stop before the watcher ends, so a signal that lands after dispatch
	// returned takes its default action instead of sitting unread in sigs.
	defer func() { signal.Stop(sigs); close(done); <-watched }()

	opts.ctx = ctx
	code := dispatch(opts)
	// Read ctx before the deferred cancel, which ends it.
	if ctx.Err() != nil {
		return 2
	}
	return code
}

// signalIgnored is signal.Ignored, indirected so a test can model a
// process started with an interrupt signal ignored.
var signalIgnored = signal.Ignored

// interruptSignals returns the signals that cancel the build: SIGINT
// and SIGTERM, minus any the process started with ignored. A background
// job of a non-interactive shell starts with SIGINT ignored, and Notify
// would un-ignore it, so a Ctrl-C meant for the foreground job would
// cancel this build.
func interruptSignals() []os.Signal {
	out := make([]os.Signal, 0, 2)
	for _, s := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		if !signalIgnored(s) {
			out = append(out, s)
		}
	}
	return out
}

// watchInterrupts turns signals into the build's two kill stages: the
// first one received on sigs calls cancel (recipes get SIGTERM and the
// grace period), the second closes force (SIGKILL at once). Later
// signals are swallowed, because exiting before every recipe group is
// reaped would orphan it. It returns when done is closed.
func watchInterrupts(
	sigs <-chan os.Signal, cancel context.CancelFunc, force chan<- struct{}, done <-chan struct{},
) {
	select {
	case <-sigs:
		cancel()
	case <-done:
		return
	}
	select {
	case <-sigs:
		close(force)
	case <-done:
		return
	}
	for {
		select {
		case <-sigs:
		case <-done:
			return
		}
	}
}
