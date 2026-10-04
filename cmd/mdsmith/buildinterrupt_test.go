package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchInterrupts_FirstCancelsSecondForces(t *testing.T) {
	sigs := make(chan os.Signal)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	force := make(chan struct{})
	done := make(chan struct{})
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		watchInterrupts(sigs, cancel, force, done)
	}()

	sigs <- os.Interrupt
	<-ctx.Done()
	select {
	case <-force:
		t.Fatal("one signal must not escalate the kill")
	default:
	}

	sigs <- os.Interrupt
	select {
	case <-force:
	case <-time.After(5 * time.Second):
		t.Fatal("second signal must close force")
	}

	// A third signal is swallowed, not a panic on a closed channel.
	sigs <- os.Interrupt
	close(done)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher must return once done closes")
	}
}

func TestWatchInterrupts_DoneBeforeAnySignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	force := make(chan struct{})
	done := make(chan struct{})
	close(done)
	watchInterrupts(make(chan os.Signal), cancel, force, done)
	require.NoError(t, ctx.Err(), "no signal, no cancel")
}

func TestWatchInterrupts_DoneAfterOneSignal(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	force := make(chan struct{})
	done := make(chan struct{})
	sigs <- os.Interrupt
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		watchInterrupts(sigs, cancel, force, done)
	}()
	<-ctx.Done()
	close(done)
	<-returned
	select {
	case <-force:
		t.Fatal("force must stay open after one signal")
	default:
	}
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}
