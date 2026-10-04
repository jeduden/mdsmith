//go:build unix

package build

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

// TestRunHook_CancelKillsHookChildren checks that a hook gets the same
// group kill as a recipe: cancelling its context (a CLI interrupt)
// must also end the background child the hook spawned, not only the
// hook's leader process.
func TestRunHook_CancelKillsHookChildren(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := writeScript(t, t.TempDir(), "spawn.sh",
		`sleep 120 & echo $! > "`+pidFile+`"; sleep 120`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	childPID := make(chan int, 1)
	go func() {
		defer cancel()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if b, err := os.ReadFile(pidFile); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
					childPID <- n
					return
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		childPID <- 0
	}()

	result := runHook(ctx, []string{script}, dir)
	pid := <-childPID
	t.Cleanup(func() {
		if t.Failed() && pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	require.NotNil(t, result)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "(interrupted)")
	require.NotZero(t, pid, "hook child pid should have been recorded")
	assert.Eventually(t, func() bool { return !processAlive(pid) },
		6*time.Second, 100*time.Millisecond, "hook child must not be orphaned")
}

// TestRunHook_KeepsMdsmithEnvironment checks that moving hooks onto
// runRecipe did not give them the hermetic recipe environment: a hook
// still sees a variable from mdsmith's own environment.
func TestRunHook_KeepsMdsmithEnvironment(t *testing.T) {
	t.Setenv("MDSMITH_HOOK_ENV_PROBE", "yes")
	script := writeScript(t, t.TempDir(), "probe.sh", `[ "$MDSMITH_HOOK_ENV_PROBE" = yes ]`)
	assert.Nil(t, runHook(context.Background(), []string{script}, t.TempDir()))
}
