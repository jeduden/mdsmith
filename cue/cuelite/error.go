package cuelite

import (
	"reflect"
	"slices"
	"strings"
)

// PathError reports a validation failure tagged with the field path
// at which it occurred. The path mirrors cuelang.org/go/cue/errors
// Error.Path() — the dotted route into the data tree where the value
// failed its constraint (for example []string{"meta", "status"}). A
// nil path marks an error not associated with a specific leaf, such as
// a bottom [Value]'s compile failure.
//
// A PathError may wrap the underlying error it was built from — the
// CUE validation error for a per-leaf failure, or a bottom Value's
// cause — reachable through [PathError.Unwrap] so errors.Is/As keep
// working against the original error or a sentinel. Its own message,
// not the wrapped cause, is the rejection; [Errors] treats a PathError
// as a leaf and does not descend into the wrapped error.
type PathError struct {
	path    []string
	msg     string
	wrapped error
}

// newPathError builds a PathError at the given field path with the
// given message, retaining cause as its wrapped error (nil when there is
// none) so errors.Is/As against the original CUE error or a sentinel
// resolve through the returned leaf. A nil or empty path produces an
// error whose Error() is the bare message, with no path prefix. The
// message must be path-free: Error() owns the single path prefix, so a
// message that already carries the path would double it.
func newPathError(path []string, msg string, cause error) *PathError {
	return &PathError{path: path, msg: msg, wrapped: cause}
}

// Unwrap returns the underlying error the PathError was built from, or
// nil when it carries none, so errors.Is/As reach the wrapped CUE error
// or sentinel. Errors does not follow this link — a PathError is a leaf
// in the per-field walk — so unwrapping is for errors.Is/As only.
func (e *PathError) Unwrap() error {
	return e.wrapped
}

// Path returns the field path the error is tagged with, or nil when
// the error is not associated with a specific leaf. It mirrors
// cue/errors Error.Path().
//
// The returned slice is a fresh copy, never the error's internal slice
// (matching cue/errors, which clones in Error.Path): a caller that
// mutates the copy cannot corrupt a later Error() render.
// slices.Clone(nil) is nil, so an unpathed error still returns nil
// rather than an empty slice.
func (e *PathError) Path() []string {
	return slices.Clone(e.path)
}

// Error renders the message prefixed by the dotted field path, or the
// bare message when the path is empty.
func (e *PathError) Error() string {
	if len(e.path) == 0 {
		return e.msg
	}
	return strings.Join(e.path, ".") + ": " + e.msg
}

// Errors enumerates the per-field failures carried by an error
// returned from [Value.Validate]. It does not depend on the concrete
// shape Validate returns, which Validate's own doc leaves unspecified.
// Whatever that shape is (today a bare [*PathError] for a single
// failing field, an errors.Join of *PathErrors for several, a
// path-free *PathError for a bottom), Errors flattens it into one
// slice, so callers iterate uniformly without type-switching on the
// result. It mirrors cuelang.org/go/cue/errors.Errors.
//
// Errors is a full error-tree walk: it descends through both join
// wrappers (Unwrap() []error) and single wrappers (Unwrap() error),
// collecting every *PathError leaf in encounter order. A *PathError
// hidden behind a fmt.Errorf("%w", …) wrapper, or a join nested inside
// such a wrapper, is therefore reported in full — not truncated to the
// first leaf an errors.As would stop at. A nil error, or an error tree
// carrying no *PathError, yields nil — never a non-nil empty slice —
// so a caller can range over the result unconditionally.
//
// This walk underpins the invariant documented on [Value.Validate]:
// every non-nil error Validate returns decomposes to at least one
// *PathError, so a consumer loop over Errors emits at least one
// diagnostic for any failing value.
func Errors(err error) []*PathError {
	if err == nil {
		return nil
	}
	var out []*PathError
	return collectPathErrors(err, out, map[error]struct{}{})
}

// collectPathErrors appends every *PathError leaf reachable from err to
// out in encounter order. A node that is itself a *PathError is a leaf
// and is appended directly (its own message, not its wrapped cause,
// being the rejection — the walk does NOT descend into a PathError's
// Unwrap). Otherwise the walk recurses through a join wrapper (Unwrap()
// []error) or a single wrapper (Unwrap() error). A node that is neither
// a *PathError nor any wrapper contributes nothing.
//
// visited records the nodes already walked, so a node reachable by more
// than one path — errors.Join sharing a leaf with a %w-wrapper of it —
// is counted once, and a cyclic Unwrap chain terminates instead of
// recursing forever. A *PathError leaf is appended before the visited
// check can dedup it only through its parents, so each distinct leaf
// pointer still yields one entry.
//
// Only a comparable node is memoized: an uncomparable concrete type (a
// slice- or map-backed error) cannot be a map key — inserting it would
// panic "hash of unhashable type". Such a node is walked WITHOUT
// memoization, so a cycle reachable only through it could recurse
// forever; in practice a cycle needs a comparable self-referencing node
// (an uncomparable value cannot equal itself by ==, so it cannot close a
// Go error chain back onto itself), which is still memoized and still
// terminates.
func collectPathErrors(err error, out []*PathError, visited map[error]struct{}) []*PathError {
	if err == nil {
		return out
	}
	comparable := reflect.TypeOf(err).Comparable()
	if comparable {
		if _, seen := visited[err]; seen {
			return out
		}
		visited[err] = struct{}{}
	}
	if pe, ok := err.(*PathError); ok {
		return append(out, pe)
	}
	switch w := err.(type) {
	case interface{ Unwrap() []error }:
		for _, leaf := range w.Unwrap() {
			out = collectPathErrors(leaf, out, visited)
		}
	case interface{ Unwrap() error }:
		out = collectPathErrors(w.Unwrap(), out, visited)
	}
	return out
}
