package include

import (
	"strings"
	"unsafe"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

// The include rule rewrites included content as strings, line by line,
// and tracks no list or block-quote containers. Its fence checks
// therefore strip all leading spaces and tabs before asking mdfence, so
// a fence nested in a list item (indented past CommonMark's three-space
// budget) is still detected and its body skipped. The mdfence rules
// apply to the stripped line unchanged: run length, matching closer
// character and length, no backtick in a backtick fence's info string,
// and only whitespace (including a CRLF line's "\r") after a closer.

// fenceBytes views s as a byte slice without copying, for mdfence's
// byte-slice API. mdfence only reads the slice and never retains it,
// so the string's bytes are never mutated and outlive the view.
func fenceBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// stepFence advances t past line, after stripping its leading
// whitespace, and reports whether line belongs to a fenced code block
// (opener, content, or closer).
func stepFence(t *mdfence.Tracker, line string) bool {
	return t.Step(fenceBytes(strings.TrimLeft(line, " \t")))
}

// opensFence reports whether line, after stripping its leading
// whitespace, opens a fenced code block.
func opensFence(line string) bool {
	_, ok := mdfence.Open(fenceBytes(strings.TrimLeft(line, " \t")))
	return ok
}
