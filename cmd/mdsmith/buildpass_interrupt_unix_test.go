//go:build unix

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForFile polls until path exists and returns its trimmed content.
func waitForFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b))
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return ""
}

// pidGone reports whether the process no longer exists.
func pidGone(pid int) bool {
	return syscall.Kill(pid, 0) == syscall.ESRCH
}

// runInterruptedBuild runs the build pass over a recipe that spawns a
// long-lived child, cancels ctx once the child's pid is recorded, and
// returns the pass exit code, its output, and the child's pid.
func runInterruptedBuild(t *testing.T) (int, string, int) {
	t.Helper()
	root := t.TempDir()
	trustRoot(t, root)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := writeShScript(t, t.TempDir(), "spawn.sh",
		`sleep 120 & echo $! > "`+pidFile+`"; sleep 120`)
	cfg := buildPassCfg("    hang:\n      command: " + script + " {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(buildPassDirective("hang", "out.txt")), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		waitForFile(t, pidFile)
		cancel()
	}()

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p},
		buildPassOpts{ctx: ctx, timeout: time.Minute, noCache: true}, &buf)
	pid, err := strconv.Atoi(waitForFile(t, pidFile))
	require.NoError(t, err)
	return code, buf.String(), pid
}

func TestRunBuildPass_CancelKillsRecipeChild(t *testing.T) {
	code, _, pid := runInterruptedBuild(t)
	assert.NotEqual(t, 0, code)
	assert.Eventually(t, func() bool { return pidGone(pid) },
		5*time.Second, 20*time.Millisecond, "recipe child must be killed")
}

func TestRunBuildPass_CancelReportsInterruptNotTimeout(t *testing.T) {
	code, out, _ := runInterruptedBuild(t)
	assert.Equal(t, 2, code)
	assert.Contains(t, out, "INTERRUPTED out.txt")
	assert.NotContains(t, out, "TIMEOUT")
}
