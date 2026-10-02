package build

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failPipeAfter makes the (n+1)th pipeFn call fail for one test.
func failPipeAfter(t *testing.T, n int) {
	t.Helper()
	old := pipeFn
	calls := 0
	pipeFn = func() (*os.File, *os.File, error) {
		calls++
		if calls > n {
			return nil, nil, errors.New("no pipes")
		}
		return old()
	}
	t.Cleanup(func() { pipeFn = old })
}

// requireDrained fails unless ro.drained closes within a second.
func requireDrained(t *testing.T, ro *recipeOutput) {
	t.Helper()
	select {
	case <-ro.drained:
	case <-time.After(time.Second):
		t.Fatal("copy goroutines did not end")
	}
}

func TestRecipeOutput_AttachFirstPipeFails(t *testing.T) {
	failPipeAfter(t, 0)
	ro := &recipeOutput{}
	err := ro.attach(&exec.Cmd{}, &strings.Builder{}, nil)
	require.ErrorContains(t, err, "no pipes")
	requireDrained(t, ro)
}

func TestRunRecipe_PipeErrorReported(t *testing.T) {
	failPipeAfter(t, 0)
	code, timedOut, err := runRecipe(context.Background(), runOpts{
		argv:    []string{"unused"},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
		stdout:  &strings.Builder{},
	})
	require.ErrorContains(t, err, "capturing recipe output")
	assert.Equal(t, -1, code)
	assert.False(t, timedOut)
}

func TestInterfaceEqual(t *testing.T) {
	b := &strings.Builder{}
	assert.True(t, interfaceEqual(b, b))
	assert.False(t, interfaceEqual(b, &strings.Builder{}))
	// Two values of one non-comparable type panic under ==.
	assert.False(t, interfaceEqual([]int{1}, []int{1}))
}

func TestExitCodeOf_NotExitError(t *testing.T) {
	assert.Equal(t, -1, exitCodeOf(nil))
	assert.Equal(t, -1, exitCodeOf(errors.New("boom")))
}

func TestExitResult(t *testing.T) {
	code, timedOut, err := exitResult(nil)
	assert.Equal(t, 0, code)
	assert.False(t, timedOut)
	require.NoError(t, err)

	want := errors.New("copy failed")
	code, timedOut, err = exitResult(want)
	assert.Equal(t, -1, code)
	assert.False(t, timedOut)
	assert.Same(t, want, err)
}

func TestOutputGate(t *testing.T) {
	var a, b strings.Builder
	g := &outputGate{}
	wa, wb := g.wrap(&a), g.wrap(&b)

	n, err := wa.Write([]byte("kept"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	g.close()
	n, err = wb.Write([]byte("dropped"))
	require.NoError(t, err)
	assert.Equal(t, 7, n, "a closed gate reports the write done so the copy keeps draining")

	assert.Equal(t, "kept", a.String())
	assert.Empty(t, b.String())
}
