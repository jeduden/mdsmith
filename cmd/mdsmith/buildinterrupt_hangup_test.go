//go:build unix || plan9

package main

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterruptSignals_IncludesHangup(t *testing.T) {
	// Closing the terminal or window, or dropping an SSH session, sends
	// SIGHUP (the "hangup" note on plan9): it must cancel the build too,
	// or the recipe groups are orphaned.
	stubSignalIgnored(t)
	assert.Contains(t, interruptSignals(), syscall.SIGHUP)

	// Under nohup SIGHUP starts ignored and must stay so.
	stubSignalIgnored(t, syscall.SIGHUP)
	assert.NotContains(t, interruptSignals(), syscall.SIGHUP)
}
