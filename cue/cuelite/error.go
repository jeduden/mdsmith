package cuelite

import "strings"

// PathError reports a validation failure tagged with the field path
// at which it occurred. The path mirrors cuelang.org/go/cue/errors
// Error.Path() — the dotted route into the data tree where the value
// failed its constraint (for example []string{"meta", "status"}). A
// nil path marks an error not associated with a specific leaf.
type PathError struct {
	path []string
	msg  string
}

// NewPathError builds a PathError at the given field path with the
// given message. A nil or empty path produces an error whose Error()
// is the bare message, with no path prefix.
func NewPathError(path []string, msg string) *PathError {
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
