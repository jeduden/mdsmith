package cuelite

import (
	"strings"
)

// PathError reports a validation failure tagged with the field path
// at which it occurred. The path mirrors cuelang.org/go/cue/errors
// Error.Path() — the dotted route into the data tree where the value
// failed its constraint (for example []string{"meta", "status"}). A
// nil path marks an error not associated with a specific leaf.
type PathError struct {
	path []string
	msg  string
}

// newPathError builds a PathError at the given field path with the
// given message. A nil or empty path produces an error whose Error()
// is the bare message, with no path prefix. The message must be
// path-free: Error() is responsible for the single path prefix, so a
// message that already carries the path would double it.
func newPathError(path []string, msg string) *PathError {
	return &PathError{path: path, msg: msg}
}

// Path returns the field path the error is tagged with, or nil when
// the error is not associated with a specific leaf. It mirrors
// cue/errors Error.Path() so the differential harness can compare
// in-house and CUE-backed error locations field by field.
func (e *PathError) Path() []string {
	return e.path
}

// Error renders the message prefixed by the dotted field path, or the
// bare message when the path is empty.
func (e *PathError) Error() string {
	if len(e.path) == 0 {
		return e.msg
	}
	return strings.Join(e.path, ".") + ": " + e.msg
}

// Errors enumerates the per-field failures carried by an error returned
// from Validate. It is THE way a consumer reads every rejecting leaf:
// Validate returns one *PathError when a single field fails and an
// errors.Join of *PathErrors when several do, and Errors flattens both
// into one slice so callers (the internal/schema validator emitting one
// MDS020 diagnostic per field, the differential harness comparing every
// rejected path) iterate uniformly without type-switching on the join
// shape. It mirrors cuelang.org/go/cue/errors.Errors.
//
// Errors is a full error-tree walk: it descends through both join
// wrappers (Unwrap() []error) and single wrappers (Unwrap() error),
// collecting every *PathError leaf in encounter order. A *PathError
// hidden behind a fmt.Errorf("%w", …) wrapper, or a join nested inside
// such a wrapper, is therefore reported in full — not truncated to the
// first leaf an errors.As would stop at. A nil error, or an error tree
// carrying no *PathError, yields nil — never a non-nil empty slice — so
// a caller can range over the result unconditionally.
//
// This walk underpins the invariant documented on Validate: every
// non-nil error Validate returns decomposes to at least one *PathError,
// so a consumer loop over Errors emits at least one diagnostic for any
// failing value.
func Errors(err error) []*PathError {
	if err == nil {
		return nil
	}
	var out []*PathError
	return collectPathErrors(err, out)
}

// collectPathErrors appends every *PathError leaf reachable from err to
// out in encounter order. A node that is itself a *PathError is a leaf
// and is appended directly (its own message, not its wrapped causes,
// being the rejection). Otherwise the walk recurses through a join
// wrapper (Unwrap() []error) or a single wrapper (Unwrap() error). A
// node that is neither a *PathError nor any wrapper contributes nothing.
func collectPathErrors(err error, out []*PathError) []*PathError {
	if err == nil {
		return out
	}
	if pe, ok := err.(*PathError); ok {
		return append(out, pe)
	}
	switch w := err.(type) {
	case interface{ Unwrap() []error }:
		for _, leaf := range w.Unwrap() {
			out = collectPathErrors(leaf, out)
		}
	case interface{ Unwrap() error }:
		out = collectPathErrors(w.Unwrap(), out)
	}
	return out
}
