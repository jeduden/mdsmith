package include

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/mdfence"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// The include rule rewrites included content as strings, line by line,
// and tracks no list or block-quote containers. Its fence checks
// therefore strip all leading spaces and tabs before asking mdfence, so
// a fence nested in a list item (indented past CommonMark's three-space
// budget) is still detected and its body skipped. The mdfence rules
// apply to the stripped line unchanged: run length, matching closer
// character and length, no backtick in a backtick fence's info string,
// and only whitespace (including a CRLF line's "\r") after a closer.
//
// The string is handed to mdfence's byte-slice API through
// util.StringToReadOnlyBytes, which views it without copying: mdfence
// only reads the slice and never retains it.

// stepFence advances t past line, after stripping its leading
// whitespace, and reports whether line belongs to a fenced code block
// (opener, content, or closer).
func stepFence(t *mdfence.Tracker, line string) bool {
	return t.Step(util.StringToReadOnlyBytes(strings.TrimLeft(line, " \t")))
}
