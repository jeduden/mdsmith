//go:build unix

package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_Build_SignalKillsRecipeTree starts `mdsmith fix` on a recipe
// that spawns a long-lived child, sends the CLI the signal once the
// child is running, and asserts the CLI exits non-zero, reports an
// interrupt (not a timeout), and leaves no recipe process behind.
func TestE2E_Build_SignalKillsRecipeTree(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := writeBuildRepo(t, "")
			pidFile := filepath.Join(dir, "child.pid")
			script := "#!/bin/sh\nsleep 120 & echo $! > \"" + pidFile + "\"\nsleep 120\ntouch \"$1\"\n"
			scriptPath := filepath.Join(dir, "spawn.sh")
			require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
			reconfigureRecipe(t, dir, "    spawn:\n      command: "+scriptPath+" {outputs}\n")
			writeFixture(t, dir, "doc.md", buildDirective("spawn", "", "out.txt"))

			cmd := exec.Command(binaryPath, "fix", "--no-color", "--build-only", "doc.md")
			cmd.Dir = dir
			cmd.Env = envWithCoverDir(coverDir)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Start())

			var childPID int
			deadline := time.Now().Add(10 * time.Second)
			for childPID == 0 && time.Now().Before(deadline) {
				if b, err := os.ReadFile(pidFile); err == nil {
					childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
				}
				time.Sleep(20 * time.Millisecond)
			}
			require.NotZero(t, childPID, "child pid should have been recorded")
			require.NoError(t, cmd.Process.Signal(sig))

			err := cmd.Wait()
			ee, ok := err.(*exec.ExitError)
			require.True(t, ok, "expected non-zero exit, got %v", err)
			assert.Equal(t, 2, ee.ExitCode(), stderr.String())
			assert.Contains(t, stderr.String(), "INTERRUPTED")
			assert.NotContains(t, stderr.String(), "TIMEOUT")
			assert.Eventually(t, func() bool { return !unixProcessAlive(childPID) },
				6*time.Second, 100*time.Millisecond, "recipe child must not be orphaned")
		})
	}
}
