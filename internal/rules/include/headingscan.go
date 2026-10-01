package include

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/mdfence"
	"github.com/jeduden/mdsmith/internal/mdhtml"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// headingScan follows the block context the include heading rewrite
// needs, one line at a time: an open fenced code block, an open HTML
// block, an open multi-line processing instruction, and whether the
// previous line is paragraph text a setext underline can turn into a
// heading. Lines inside a fence, HTML block, or processing instruction
// are never headings. The zero headingScan is at a block boundary.
//
// It tracks no list or block-quote containers; see fence.go for how
// fence lines are read without them.
type headingScan struct {
	fence mdfence.Tracker
	html  mdhtml.Kind
	pi    bool
	para  bool
}

// step advances the scan past line and returns its heading level (0 when
// line is no heading). setext reports that line is a setext underline,
// whose heading text is the previous line.
func (h *headingScan) step(line string) (level int, setext bool) {
	b := util.StringToReadOnlyBytes(line)
	wasPara := h.para
	h.para = false
	switch {
	case h.html != mdhtml.None:
		if mdhtml.Closes(b, h.html) {
			h.html = mdhtml.None
		}
		return 0, false
	case h.pi:
		h.pi = strings.TrimSpace(line) != "?>"
		return 0, false
	case stepFence(&h.fence, line):
		return 0, false
	}
	if level := setextLevel(line); level > 0 {
		if wasPara {
			return level, true
		}
		// A "---" or "===" line that follows no paragraph is a thematic
		// break or a lone paragraph line; it is kept conservatively out
		// of setext heading text.
		return 0, false
	}
	if atxRe.MatchString(line) {
		return atxLevel(line), false
	}
	if strings.TrimSpace(line) == "" {
		return 0, false
	}
	if opened, closed := piStart(line); opened {
		h.pi = !closed
		return 0, false
	}
	if k := mdhtml.Open(b, wasPara); k != mdhtml.None {
		if !mdhtml.Closes(b, k) {
			h.html = k
		}
		return 0, false
	}
	h.para = true
	return 0, false
}

// atxLevel returns the number of leading '#' of a line atxRe matched.
func atxLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	return n
}

// piStart mirrors the processing-instruction block parser in
// pkg/markdown/pi_parser.go: up to three spaces, "<?", then a non-empty
// name (ending at whitespace or "?>"). opened reports a PI start; closed
// reports that "?>" also appears on the line, so the PI ends there. A
// multi-line PI ends on a line that is "?>" alone. A "<?" with no name
// is left to mdhtml, which reads it as an HTML block of type 3.
func piStart(line string) (opened, closed bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || !strings.HasPrefix(trimmed, "<?") {
		return false, false
	}
	rest := strings.TrimRight(trimmed[2:], "\r\n")
	if rest == "" || rest[0] == ' ' || rest[0] == '\t' || strings.HasPrefix(rest, "?>") {
		return false, false
	}
	return true, strings.Contains(strings.TrimRight(trimmed, " \t\r\n"), "?>")
}
