//go:build !unix

package build

import "os/exec"

// killGroupUntil is killGroup on platforms whose kill has no grace
// period (Windows, plan9, js/wasm): there is nothing for a second
// interrupt to cut short, so force is ignored.
func killGroupUntil(cmd *exec.Cmd, _ <-chan struct{}) { killGroup(cmd) }
