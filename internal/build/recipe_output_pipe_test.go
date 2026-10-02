//go:build unix || windows

package build

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecipeOutput_FileWritersGoDirect(t *testing.T) {
	cmd := &exec.Cmd{}
	ro := &recipeOutput{}
	require.NoError(t, ro.attach(cmd, nil, nil))
	assert.Same(t, os.Stderr, cmd.Stdout, "nil stdout means os.Stderr, unpiped")
	assert.Same(t, os.Stderr, cmd.Stderr)
	assert.Empty(t, ro.readers)
	requireDrained(t, ro)
}

func TestRecipeOutput_AttachSecondPipeFailsClosesFirst(t *testing.T) {
	// The first pipeFn call is the real os.Pipe, which js/wasm and
	// wasip1 lack, so this test lives in the unix || windows file.
	failPipeAfter(t, 1)
	ro := &recipeOutput{}
	err := ro.attach(&exec.Cmd{}, &strings.Builder{}, &strings.Builder{})
	require.ErrorContains(t, err, "no pipes")
	// The first pipe's copy goroutine must end: its read end is closed.
	requireDrained(t, ro)
}

func TestRecipeOutput_OneWriterSharesOnePipe(t *testing.T) {
	cmd := &exec.Cmd{}
	out := &lockedBuffer{}
	ro := &recipeOutput{}
	require.NoError(t, ro.attach(cmd, out, out))
	t.Cleanup(ro.closeChildEnds)
	require.Len(t, ro.readers, 1)
	assert.Same(t, cmd.Stdout, cmd.Stderr)

	_, err := cmd.Stdout.Write([]byte("a"))
	require.NoError(t, err)
	_, err = cmd.Stderr.Write([]byte("b"))
	require.NoError(t, err)
	ro.closeChildEnds()
	requireDrained(t, ro)
	assert.Equal(t, "ab", out.String())
	require.NoError(t, ro.err())
}

func TestRecipeOutput_TwoWritersGetTwoPipes(t *testing.T) {
	cmd := &exec.Cmd{}
	o, e := &lockedBuffer{}, &lockedBuffer{}
	ro := &recipeOutput{}
	require.NoError(t, ro.attach(cmd, o, e))
	require.Len(t, ro.readers, 2)

	_, err := cmd.Stdout.Write([]byte("out"))
	require.NoError(t, err)
	_, err = cmd.Stderr.Write([]byte("err"))
	require.NoError(t, err)
	ro.closeChildEnds()
	requireDrained(t, ro)
	assert.Equal(t, "out", o.String())
	assert.Equal(t, "err", e.String())
}

// failingWriter rejects every write.
type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestRecipeOutput_WriterErrorRecordedAndPipeClosed(t *testing.T) {
	cmd := &exec.Cmd{}
	want := errors.New("disk full")
	ro := &recipeOutput{}
	require.NoError(t, ro.attach(cmd, failingWriter{want}, nil))
	child := cmd.Stdout.(*os.File)

	_, err := child.WriteString("x")
	require.NoError(t, err)
	requireDrained(t, ro)
	assert.ErrorIs(t, ro.err(), want)
	// The read end is closed, so the child's next write fails rather
	// than blocking once the pipe buffer fills.
	_, err = child.WriteString("y")
	require.Error(t, err)
	ro.closeChildEnds()
}

func TestRecipeOutput_AbandonEndsCopyWhileWriterOpen(t *testing.T) {
	cmd := &exec.Cmd{}
	ro := &recipeOutput{}
	require.NoError(t, ro.attach(cmd, &lockedBuffer{}, nil))
	t.Cleanup(ro.closeChildEnds)
	// The write end stays open, as a survivor would hold it.
	ro.abandon()
	requireDrained(t, ro)
}

func TestTimeoutResult(t *testing.T) {
	old := reapWait
	reapWait = 50 * time.Millisecond
	t.Cleanup(func() { reapWait = old })

	// Output still held open: timeoutResult abandons it after reapWait.
	ro := &recipeOutput{}
	require.NoError(t, ro.attach(&exec.Cmd{}, &lockedBuffer{}, nil))
	t.Cleanup(ro.closeChildEnds)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code, timedOut, err := timeoutResult(ctx, ro, nil)
	assert.Equal(t, -1, code)
	assert.True(t, timedOut)
	require.ErrorContains(t, err, "recipe cancelled")
	requireDrained(t, ro)

	// Output already drained: the deadline is reported as a timeout.
	ro = &recipeOutput{}
	require.NoError(t, ro.attach(&exec.Cmd{}, nil, nil))
	dctx, dcancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer dcancel()
	<-dctx.Done()
	_, timedOut, err = timeoutResult(dctx, ro, nil)
	assert.True(t, timedOut)
	require.ErrorContains(t, err, "recipe timed out")
}

// lockedBuffer is a strings.Builder safe to read while a copy
// goroutine may still write to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
