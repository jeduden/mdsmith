package cuelite

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPathError(t *testing.T) {
	err := newPathError([]string{"meta", "status"}, "value out of range")
	require.NotNil(t, err)
	assert.Equal(t, []string{"meta", "status"}, err.Path())
	assert.Equal(t, "meta.status: value out of range", err.Error())
}

func TestNewPathError_emptyPath(t *testing.T) {
	err := newPathError(nil, "front matter does not satisfy schema")
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

func TestPathError_errorsAs(t *testing.T) {
	var wrapped error = newPathError([]string{"x"}, "boom")
	var pe *PathError
	require.True(t, errors.As(wrapped, &pe))
	assert.Equal(t, []string{"x"}, pe.Path())
}

func TestErrors(t *testing.T) {
	t.Run("nil error yields nil", func(t *testing.T) {
		assert.Nil(t, Errors(nil))
	})
	t.Run("foreign error yields nil", func(t *testing.T) {
		assert.Nil(t, Errors(errors.New("not a path error")))
	})
	t.Run("single bare PathError yields one", func(t *testing.T) {
		got := Errors(newPathError([]string{"a"}, "boom"))
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
	t.Run("joined PathErrors are all collected in order", func(t *testing.T) {
		joined := errors.Join(
			newPathError([]string{"a"}, "x"),
			newPathError([]string{"b"}, "y"),
		)
		got := Errors(joined)
		require.Len(t, got, 2)
		assert.Equal(t, []string{"a"}, got[0].Path())
		assert.Equal(t, []string{"b"}, got[1].Path())
	})
	t.Run("a join mixing a foreign error keeps only the PathErrors", func(t *testing.T) {
		joined := errors.Join(
			newPathError([]string{"a"}, "x"),
			errors.New("foreign"),
		)
		got := Errors(joined)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
	t.Run("leaves behind a single wrapper are found", func(t *testing.T) {
		// fmt.Errorf("%w", pe) is an Unwrap() error wrapper, not a join;
		// Errors must recurse through it to reach the leaf.
		wrapped := fmt.Errorf("context: %w", newPathError([]string{"a"}, "boom"))
		got := Errors(wrapped)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"a"}, got[0].Path())
	})
	t.Run("leaves behind a wrapped join are all found in order", func(t *testing.T) {
		// A join hidden behind a single wrapper: errors.As stops at the
		// first leaf, so a walk that does not recurse past the wrapper into
		// the join's branches would report only one of the two.
		pe1 := newPathError([]string{"a"}, "x")
		pe2 := newPathError([]string{"b"}, "y")
		wrapped := fmt.Errorf("ctx: %w", errors.Join(pe1, pe2))
		got := Errors(wrapped)
		require.Len(t, got, 2)
		assert.Equal(t, []string{"a"}, got[0].Path())
		assert.Equal(t, []string{"b"}, got[1].Path())
	})
	t.Run("nested joins flatten in encounter order", func(t *testing.T) {
		nested := errors.Join(
			errors.Join(
				newPathError([]string{"a"}, "x"),
				newPathError([]string{"b"}, "y"),
			),
			newPathError([]string{"c"}, "z"),
		)
		got := Errors(nested)
		require.Len(t, got, 3)
		assert.Equal(t, []string{"a"}, got[0].Path())
		assert.Equal(t, []string{"b"}, got[1].Path())
		assert.Equal(t, []string{"c"}, got[2].Path())
	})
}
