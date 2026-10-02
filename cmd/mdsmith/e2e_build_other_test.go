//go:build !unix

package main_test

// canProbeProcess is false off Unix: there is no signal-0 existence
// probe, so the timeout/process-group test skips rather than trusting
// the stub below.
const canProbeProcess = false

// unixProcessAlive is a stub for Windows, js/wasm, wasip1, and plan9.
// It only satisfies the compiler so the shared hardening test file
// builds on every target; the caller skips when !canProbeProcess.
func unixProcessAlive(int) bool { return false }
