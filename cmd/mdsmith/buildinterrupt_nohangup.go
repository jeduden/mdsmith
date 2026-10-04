//go:build !unix && !plan9

package main

import "os"

// hangupSignals is empty off Unix and plan9: Windows and js/wasm have
// no hangup signal to catch.
var hangupSignals []os.Signal
