package cuelite

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPathError(t *testing.T) {
	t.Run("path and message, nil cause", func(t *testing.T) {
		err := newPathError([]string{"meta", "status"}, "value out of range", nil)
		require.NotNil(t, err)
		assert.Equal(t, []string{"meta", "status"}, err.Path())
		assert.Equal(t, "meta.status: value out of range", err.Error())
		assert.NoError(t, err.Unwrap(), "a nil cause leaves Unwrap nil")
	})
	t.Run("non-nil cause is retained for errors.Is/As", func(t *testing.T) {
		// The single consolidated constructor still wraps a cause so the
		// returned leaf resolves errors.Is against it.
		cause := errors.New("underlying")
		err := newPathError([]string{"x"}, "boom", cause)
		assert.Same(t, cause, err.Unwrap(), "Unwrap returns the retained cause")
		assert.True(t, errors.Is(err, cause), "errors.Is reaches the wrapped cause")
	})
}

func TestNewPathError_emptyPath(t *testing.T) {
	err := newPathError(nil, "front matter does not satisfy schema", nil)
	require.NotNil(t, err)
	assert.Nil(t, err.Path())
	assert.Equal(t, "front matter does not satisfy schema", err.Error())
}

func TestPathError_Error(t *testing.T) {
	t.Run("with path", func(t *testing.T) {
		err := &PathError{path: []string{"tags", "1"}, msg: "must be a string"}
		assert.Equal(t, "tags.1: must be a string", err.Error())
	})
	t.Run("without path", func(t *testing.T) {
		err := &PathError{msg: "bare message"}
		assert.Equal(t, "bare message", err.Error())
	})
}

func TestPathError_Path(t *testing.T) {
	err := &PathError{path: []string{"a", "b"}}
	assert.Equal(t, []string{"a", "b"}, err.Path())
}

func TestPathError_Path_returnsCopy(t *testing.T) {
	// Path() must not alias the error's internal slice: a caller that
	// mutates the returned slice (the harness collects leaf.Path() into its
	// own structures) must not corrupt a later Error() render.
	err := newPathError([]string{"meta", "status"}, "boom", nil)
	got := err.Path()
	require.Len(t, got, 2)
	got[0] = "MUTATED"
	assert.Equal(t, "meta.status: boom", err.Error(),
		"mutating the returned path must not change Error() output")
	assert.Equal(t, []string{"meta", "status"}, err.Path(),
		"a second Path() call must still see the original field path")
}

func TestPathError_Unwrap(t *testing.T) {
	t.Run("returns the retained cause", func(t *testing.T) {
		cause := errors.New("cause")
		err := newPathError([]string{"a"}, "boom", cause)
		assert.Same(t, cause, err.Unwrap())
	})
	t.Run("returns nil when there is no cause", func(t *testing.T) {
		err := newPathError([]string{"a"}, "boom", nil)
		assert.NoError(t, err.Unwrap())
	})
}

func TestPathError_errorsAs(t *testing.T) {
	var wrapped error = newPathError([]string{"x"}, "boom", nil)
	var pe *PathError
	require.True(t, errors.As(wrapped, &pe))
	assert.Equal(t, []string{"x"}, pe.Path())
}

// nilUnwrapper is an error whose Unwrap() error returns nil — a wrapper
// that declares the single-cause shape but carries no cause. It exercises
// the recursion's nil guard in collectPathErrors.
type nilUnwrapper struct{}

func (nilUnwrapper) Error() string { return "wrapper with no cause" }
func (nilUnwrapper) Unwrap() error { return nil }

func TestErrors(t *testing.T) {
	t.Run("nil error yields nil", func(t *testing.T) {
		assert.Nil(t, Errors(nil))
	})
	t.Run("foreign error yields nil", func(t *testing.T) {
		assert.Nil(t, Errors(errors.New("not a path error")))
	})
	t.Run("single bare PathError yields one", func(t *testing.T) {
		got := Errors(newPathError([]string{"a"}, "boom", nil))
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
	t.Run("joined PathErrors are all collected in order", func(t *testing.T) {
		joined := errors.Join(
			newPathError([]string{"a"}, "x", nil),
			newPathError([]string{"b"}, "y", nil),
		)
		got := Errors(joined)
		require.Len(t, got, 2)
		assert.Equal(t, []string{"a"}, got[0].Path())
		assert.Equal(t, []string{"b"}, got[1].Path())
	})
	t.Run("a join mixing a foreign error keeps only the PathErrors", func(t *testing.T) {
		joined := errors.Join(
			newPathError([]string{"a"}, "x", nil),
			errors.New("foreign"),
		)
		got := Errors(joined)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
	t.Run("nested joins flatten in encounter order", func(t *testing.T) {
		nested := errors.Join(
			errors.Join(
				newPathError([]string{"a"}, "x", nil),
				newPathError([]string{"b"}, "y", nil),
			),
			newPathError([]string{"c"}, "z", nil),
		)
		got := Errors(nested)
		require.Len(t, got, 3)
		assert.Equal(t, []string{"a"}, got[0].Path())
		assert.Equal(t, []string{"b"}, got[1].Path())
		assert.Equal(t, []string{"c"}, got[2].Path())
	})
}

// cyclicError is a single-cause wrapper whose Unwrap() error points back
// at itself — a degenerate Unwrap chain. It exists to prove the tree walk
// terminates on a cycle rather than recursing forever.
type cyclicError struct{ self *cyclicError }

func (c *cyclicError) Error() string { return "cyclic" }
func (c *cyclicError) Unwrap() error { return c.self }

func TestErrors_sharedLeafCountedOnce(t *testing.T) {
	// errors.Join(verr, fmt.Errorf("wrap: %w", verr)) reaches the same
	// leaf twice; a naive walk double-counts it. The walk must visit each
	// node at most once, so one underlying leaf yields exactly one entry.
	pe := newPathError([]string{"a"}, "boom", nil)
	joined := errors.Join(pe, fmt.Errorf("wrap: %w", pe))
	got := Errors(joined)
	require.Len(t, got, 1, "a shared leaf must be reported once, not twice")
	assert.Equal(t, []string{"a"}, got[0].Path())
}

func TestErrors_cyclicChainTerminates(t *testing.T) {
	// A self-referential Unwrap chain must not loop forever. The walk
	// records visited nodes and stops; it finds no PathError, so yields nil.
	c := &cyclicError{}
	c.self = c
	assert.Nil(t, Errors(c))
}

func TestErrors_pathErrorIsALeaf(t *testing.T) {
	// A *PathError that itself wraps an error is a leaf: the walk reports
	// the PathError once and does NOT descend into its wrapped cause, so a
	// wrapped *PathError is not re-walked into extra entries.
	inner := newPathError([]string{"inner"}, "inner boom", nil)
	outer := newPathError([]string{"outer"}, "outer boom", inner)
	got := Errors(outer)
	require.Len(t, got, 1, "a wrapping PathError is one leaf, not two")
	assert.Equal(t, []string{"outer"}, got[0].Path())
}

// sliceErr is an error whose concrete type is a slice — uncomparable, so
// using it as a map key panics ("hash of unhashable type"). It exists to
// prove the Errors walk memoizes only comparable nodes.
type sliceErr []string

func (s sliceErr) Error() string { return strings.Join(s, ",") }

func TestErrors_uncomparableNodeDoesNotPanic(t *testing.T) {
	t.Run("a bare uncomparable error is walked without panicking", func(t *testing.T) {
		// Inserting an uncomparable node into the visited map would panic;
		// the walk must skip memoizing it and still return (it carries no
		// *PathError leaf, so the result is nil).
		assert.NotPanics(t, func() {
			assert.Nil(t, Errors(sliceErr{"a", "b"}))
		})
	})
	t.Run("an uncomparable error joined with a leaf collects the leaf", func(t *testing.T) {
		// errors.Join holds an uncomparable branch beside a *PathError leaf;
		// the walk must collect the leaf without panicking on the branch.
		pe := newPathError([]string{"x"}, "boom", nil)
		joined := errors.Join(sliceErr{"a"}, pe)
		var got []*PathError
		assert.NotPanics(t, func() {
			got = Errors(joined)
		})
		require.Len(t, got, 1)
		assert.Equal(t, []string{"x"}, got[0].Path())
	})
}

// TestErrors_wrappers covers the single-wrapper (Unwrap() error) leg of the
// tree walk: a leaf or a nested join hidden behind a fmt.Errorf("%w", …)
// wrapper, and a wrapper that unwraps to nil.
func TestErrors_wrappers(t *testing.T) {
	t.Run("leaves behind a single wrapper are found", func(t *testing.T) {
		// fmt.Errorf("%w", pe) is an Unwrap() error wrapper, not a join;
		// Errors must recurse through it to reach the leaf.
		wrapped := fmt.Errorf("context: %w", newPathError([]string{"a"}, "boom", nil))
		got := Errors(wrapped)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
	t.Run("leaves behind a wrapped join are all found in order", func(t *testing.T) {
		// A join hidden behind a single wrapper: errors.As stops at the
		// first leaf, so a walk that does not recurse past the wrapper into
		// the join's branches would report only one of the two.
		pe1 := newPathError([]string{"a"}, "x", nil)
		pe2 := newPathError([]string{"b"}, "y", nil)
		wrapped := fmt.Errorf("ctx: %w", errors.Join(pe1, pe2))
		got := Errors(wrapped)
		require.Len(t, got, 2)
		assert.Equal(t, []string{"a"}, got[0].Path())
		assert.Equal(t, []string{"b"}, got[1].Path())
	})
	t.Run("a single wrapper that unwraps to nil contributes nothing", func(t *testing.T) {
		// A custom Unwrap() error returning nil (a wrapper with no cause)
		// must not panic the walk: the recursion's nil guard short-circuits
		// and the node contributes no leaf.
		assert.Nil(t, Errors(nilUnwrapper{}))
	})
}
