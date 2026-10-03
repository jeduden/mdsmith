package mdsmith

import (
	"errors"

	"github.com/jeduden/mdsmith/internal/refactor"
)

// ErrNothingToRename is matched (errors.Is) by the error Session.Rename
// returns when the heading or label exists but renaming it has no
// effect — a heading or label renamed to its own text, or a heading
// renamed to the visible text it already renders as. It is a
// harmless outcome a host may ignore, unlike a missing symbol or a
// conflict; the CLI exits 1 for it. It is the engine's own sentinel,
// so the error Session.Rename returns matches it unwrapped.
var ErrNothingToRename = refactor.ErrNothingToRename

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
