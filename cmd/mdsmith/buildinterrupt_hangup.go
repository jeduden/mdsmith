//go:build unix || plan9

package main

import (
	"os"
	"syscall"
)

// hangupSignals extends interruptSignals with SIGHUP on Unix and the
// "hangup" note on plan9 (syscall.SIGHUP there): a closed terminal or
// window, or a dropped SSH session, must reap the recipe groups too.
// Recipes lead their own process group (Unix) or note group (plan9),
// so the hangup that ends mdsmith never reaches them.
var hangupSignals = []os.Signal{syscall.SIGHUP}
