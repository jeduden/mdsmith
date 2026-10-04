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
// cancelling its context, so each running recipe's process group is
// killed before mdsmith exits; recipes run in their own group and would
// otherwise survive the terminal's interrupt. A second signal escalates:
// the Unix kill skips the rest of its SIGTERM grace and sends SIGKILL
// (buildexec.WithForceKill), so mdsmith still reaps every recipe but
// does not make an impatient user wait. The handler covers only a pass
// that can start a recipe or hook: everywhere else, the lint-fix pass
// included, the signal keeps its default action and ends mdsmith at
// once. An interrupted pass exits 2, whatever a hook it cut short
// returned.
func runBuildPassInterruptible(
	cfg *config.Config, cfgPath string, files []string, opts buildPassOpts, w io.Writer,
) int {
	if !opts.runsProcesses() {
		return runBuildPass(cfg, cfgPath, files, opts, w)
	}
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	force := make(chan struct{})
	ctx, cancel := context.WithCancel(buildexec.WithForceKill(opts.context(), force))
	defer cancel()
	done := make(chan struct{})
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		watchInterrupts(sigs, cancel, force, done)
	}()
	defer func() { close(done); <-watched }()

	opts.ctx = ctx
	code := runBuildPass(cfg, cfgPath, files, opts, w)
	// Read ctx before the deferred cancel, which ends it.
	if ctx.Err() != nil {
		return 2
	}
	return code
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
