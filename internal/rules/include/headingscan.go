package include

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdfence"
	"github.com/jeduden/mdsmith/internal/mdhtml"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// paraKind is the paragraph, if any, that the previous line left open.
type paraKind uint8

const (
	// paraNone: no paragraph is open.
	paraNone paraKind = iota
	// paraRoot: a document-level paragraph. An underline after it turns
	// its lines into a setext heading.
	paraRoot
	// paraContainer: a paragraph that a list item or block quote line
	// opened, plus its lazy continuation lines. The scan reads no
	// underline as its setext underline: "---" is a thematic break that
	// ends the container, and "===" is one more lazy line.
	paraContainer
)

// headingScan follows the block context the include heading rewrite
// needs, one line at a time: an open fenced code block, an open HTML
// block, an open multi-line processing instruction, and the paragraph
// the previous line left open, which decides whether a setext underline
// turns it into a heading. Lines inside a fence, HTML block, or
// processing instruction are never headings. The zero headingScan is at
// a block boundary.
//
// It tracks no list or block-quote nesting: a list item or block quote
// line only marks the paragraph it opens as one a document-level
// underline cannot turn into a heading. See fence.go for how fence
// lines are read without containers.
type headingScan struct {
	fence mdfence.Tracker
	html  mdhtml.Kind
	pi    bool
	para  paraKind
	// paraLines counts the lines of the open paraRoot paragraph.
	paraLines int
}

// step advances the scan past line and returns its heading level (0 when
// line is no heading). text is the number of lines before line that
// form a setext heading's text when line is its underline, else 0.
func (h *headingScan) step(line string) (level, text int) {
	b := util.StringToReadOnlyBytes(line)
	wasPara, wasLines := h.para, h.paraLines
	h.para, h.paraLines = paraNone, 0
	switch {
	case h.html != mdhtml.None:
		if mdhtml.Closes(b, h.html) {
			h.html = mdhtml.None
		}
		return 0, 0
	case h.pi:
		h.pi = strings.TrimSpace(line) != "?>"
		return 0, 0
	case stepFence(&h.fence, line):
		return 0, 0
	}
	if level := setextLevel(line); level > 0 {
		if wasPara == paraRoot {
			return level, wasLines
		}
		// Not after a document-level paragraph. A thematic break or an
		// empty list item ("-") opens no paragraph. Any other underline
		// ("===", "--") is paragraph text: one more lazy line of an open
		// container paragraph, or the first line of a new paragraph.
		switch {
		case lint.IsThematicBreak(b) || lint.StartsListItem(b):
		case wasPara == paraContainer:
			h.para = paraContainer
		default:
			h.para, h.paraLines = paraRoot, 1
		}
		return 0, 0
	}
	if atxRe.MatchString(line) {
		return atxLevel(line), 0
	}
	if strings.TrimSpace(line) == "" {
		return 0, 0
	}
	if opened, closed := piStart(line); opened {
		h.pi = !closed
		return 0, 0
	}
	if k := mdhtml.Open(b, wasPara != paraNone); k != mdhtml.None {
		if !mdhtml.Closes(b, k) {
			h.html = k
		}
		return 0, 0
	}
	h.para = nextPara(b, wasPara)
	if h.para == paraRoot {
		h.paraLines = 1
		if wasPara == paraRoot {
			h.paraLines = wasLines + 1
		}
	}
	return 0, 0
}

// nextPara classifies a non-blank line that the scan found to be no
// heading, fence, HTML block, or processing instruction, given the
// paragraph prev the line before left open. A line that cannot
// interrupt the open paragraph continues it (lazily, for a container
// paragraph). Otherwise indented code and a thematic break open no
// paragraph, a list item or block quote line opens a container one, and
// any other line opens a document-level one.
func nextPara(b []byte, prev paraKind) paraKind {
	thematic := lint.IsThematicBreak(b)
	indent := 0
	for indent < len(b) && b[indent] == ' ' {
		indent++
	}
	quote := indent <= 3 && indent < len(b) && b[indent] == '>'
	// A list marker line interrupts a paragraph only when the marker
	// could open a list there (StartsInterruptingBlock: content, and an
	// ordered start of 1); ATX, fence, and HTML lines never reach here.
	interrupts := thematic || quote || lint.StartsListItem(b) && lint.StartsInterruptingBlock(b)
	switch {
	case prev != paraNone && !interrupts:
		return prev
	case thematic:
		return paraNone
	case indent >= 4 || indent < len(b) && b[indent] == '\t':
		// Four columns of indentation (a tab reaches column four) open
		// an indented code block when no paragraph is open.
		return paraNone
	case quote || lint.StartsListItem(b):
		return paraContainer
	}
	return paraRoot
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
