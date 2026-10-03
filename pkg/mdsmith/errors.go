package mdsmith

import (
	"errors"

	"github.com/jeduden/mdsmith/internal/refactor"
)

// ErrNothingToRename is matched (errors.Is) by the error Session.Rename
// returns when the heading or label exists but renaming it would change
// no byte — a heading or label renamed to its own text. It is a
// harmless outcome a host may ignore, unlike a missing symbol or a
// conflict; the CLI exits 1 for it.
var ErrNothingToRename = errors.New("nothing to rename")

// ErrorCodeNothingToRename is ErrorCode's value for an error matching
// ErrNothingToRename. The WASM binding sets it as the rejected Error's
// `code`, so a JS host can tell the no-op apart without matching text.
const ErrorCodeNothingToRename = "nothing-to-rename"

// ErrorCode returns the stable machine-readable code for an engine
// error, or "" when the error has none. Codes never change once
// published; the message text may.
func ErrorCode(err error) string {
	if errors.Is(err, ErrNothingToRename) {
		return ErrorCodeNothingToRename
	}
	return ""
}

// nothingToRenameError carries refactor's no-op rename outcome across
// the public API: its message is the engine's ("nothing to rename for
// heading \"Setup\""), and it matches the public ErrNothingToRename.
type nothingToRenameError struct{ err error }

func (e nothingToRenameError) Error() string { return e.err.Error() }

// Is makes errors.Is(err, ErrNothingToRename) hold.
func (e nothingToRenameError) Is(target error) bool { return target == ErrNothingToRename }

// Unwrap exposes the engine's NothingToRenameError.
func (e nothingToRenameError) Unwrap() error { return e.err }

// publicNothingToRename wraps err in nothingToRenameError when it is
// refactor's no-op outcome, and returns it unchanged otherwise.
func publicNothingToRename(err error) error {
	if errors.Is(err, refactor.ErrNothingToRename) {
		return nothingToRenameError{err: err}
	}
	return err
}
