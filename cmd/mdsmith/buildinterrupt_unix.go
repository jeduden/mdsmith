//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// hangupSignals extends interruptSignals with SIGHUP on Unix: a closed
// terminal or a dropped SSH session must reap the recipe groups too.
var hangupSignals = []os.Signal{syscall.SIGHUP}

// killSelf sends s to this process. It is a var so a test can observe
// the raise instead of dying of it.
var killSelf = func(s syscall.Signal) error { return syscall.Kill(syscall.Getpid(), s) }

// raiseWait is how long raiseSignal gives a self-sent signal to end
// the process before it returns to main's fallback exit. It is a var
// so a test need not wait.
var raiseWait = time.Second

// raiseSignal restores sig's default action and sends it to this
// process, so mdsmith ends by that signal. SIGINT, SIGTERM, and SIGHUP
// all terminate by default.
func raiseSignal(sig os.Signal) {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return
	}
	signal.Reset(s)
	_ = killSelf(s)
	time.Sleep(raiseWait)
}
