package lsp

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/rule"
)

func TestWatchParentProcessFiresOnDeadParent(t *testing.T) {
	t.Parallel()
	var fired atomic.Bool
	watchParentProcess(context.Background(), 4321, time.Millisecond,
		func(int) bool { return false },
		func() { fired.Store(true) })
	assert.True(t, fired.Load(), "onDead must fire once the parent is gone")
}

func TestWatchParentProcessStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var fired atomic.Bool
	watchParentProcess(ctx, 4321, time.Hour,
		func(int) bool { return true },
		func() { fired.Store(true) })
	assert.False(t, fired.Load(), "a canceled watchdog must not fire onDead")
}

func TestStartParentWatchExitsWhenParentDies(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.parentInterval = time.Millisecond
	s.parentAlive = func(int) bool { return false }
	exited := make(chan struct{})
	s.onParentExit = func() { close(exited) }

	pid := 4321
	s.startParentWatch(&pid)

	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not exit when the parent died")
	}
}

func TestStartParentWatchNoopWithoutPID(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.parentInterval = time.Millisecond
	s.parentAlive = func(int) bool {
		t.Error("liveness probe must not run when no processId was sent")
		return false
	}
	s.startParentWatch(nil)
	zero := 0
	s.startParentWatch(&zero)
	time.Sleep(20 * time.Millisecond)
}

func TestStartParentWatchNoExitWhileParentAlive(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.parentInterval = time.Millisecond
	s.parentAlive = func(int) bool { return true }
	var exited atomic.Bool
	s.onParentExit = func() { exited.Store(true) }

	pid := 4321
	s.startParentWatch(&pid)
	time.Sleep(30 * time.Millisecond)
	assert.False(t, exited.Load(), "must not exit while the parent is alive")
}
