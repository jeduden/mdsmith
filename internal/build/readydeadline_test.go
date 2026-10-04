package build

import (
	"context"
	"testing"
	"time"
)

// readyDeadline is a context whose deadline passes once a test's ready
// check holds, not after a fixed time, so the timeout kill never lands
// before a slow machine has run the recipe far enough for the test to
// mean anything. Err reports context.DeadlineExceeded, as a timeout
// does. at is when the deadline passed; read it after Done is closed.
type readyDeadline struct {
	context.Context
	done chan struct{}
	at   time.Time
}

func (c *readyDeadline) Done() <-chan struct{} { return c.done }

func (c *readyDeadline) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

// deadlineWhen returns a readyDeadline that passes once ready reports
// true, or after 30 s if it never does.
func deadlineWhen(t *testing.T, ready func() bool) *readyDeadline {
	t.Helper()
	c := &readyDeadline{Context: context.Background(), done: make(chan struct{})}
	go func() {
		defer close(c.done)
		limit := time.After(30 * time.Second)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for !ready() {
			select {
			case <-tick.C:
			case <-limit:
				c.at = time.Now()
				return
			}
		}
		c.at = time.Now()
	}()
	return c
}
