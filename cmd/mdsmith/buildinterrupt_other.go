//go:build !unix

package main

import "os"

// brokenPipeSignals is empty off Unix: a write to a broken pipe there
// returns an error instead of ending the process.
var brokenPipeSignals []os.Signal

// raiseSignal is a no-op off Unix: Windows cannot re-raise a console
// interrupt and plan9 has no default-action raise, so main exits with
// the run's code, 2 for an interrupted dispatch.
func raiseSignal(os.Signal) {}
