//go:build !unix && !windows

package lsp

// processAlive on platforms without a unix signal API or the Windows
// process API — js/wasm, wasip1, plan9. These targets never spawn the
// server as an editor child with a real parent PID to watch (the WASM
// build does not even import this package), so there is nothing to reap.
// Reporting "alive" makes the watchdog a guaranteed no-op rather than a
// source of spurious self-exit, and keeps the package compiling for
// every GOOS. The earlier `//go:build !windows` tag silently relied on
// syscall.Kill's ENOSYS stub on js/wasm and failed to compile on plan9;
// this explicit fallback covers both.
func processAlive(pid int) bool {
	return true
}
