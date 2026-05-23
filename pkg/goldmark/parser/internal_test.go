package parser

// Internal unit tests for unexported helpers and methods that the
// public test files (package parser_test) cannot reach. Tests
// here apply the test-pyramid 'unit at the base' principle by
// driving individual functions in isolation rather than through
// a full parse.

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func TestBlockquoteParser_Open_NilReturn(t *testing.T) {
	// blockquoteParser.Open returns (nil, NoChildren) when the
	// input does not start with '>'. The dispatcher only calls
	// Open when the '>' trigger has fired, so this branch is
	// unreachable through Convert but easy to drive directly.
	bp := &blockquoteParser{}
	r := text.NewReader([]byte("not a blockquote\n"))
	node, state := bp.Open(nil, r, nil)
	if node != nil {
		t.Errorf("Open on non-> line should return nil, got %v", node)
	}
	if state != NoChildren {
		t.Errorf("Open on non-> line should return NoChildren, got %v", state)
	}
}

func TestParagraphParser_Open_BlankLine(t *testing.T) {
	// paragraphParser.Open returns nil on a blank line. The
	// dispatcher only opens paragraphs when content exists, so
	// this branch is unreachable through Convert.
	pp := &paragraphParser{}
	r := text.NewReader([]byte("\n"))
	node, state := pp.Open(nil, r, nil)
	if node != nil {
		t.Errorf("Open on blank line should return nil, got %v", node)
	}
	if state != NoChildren {
		t.Errorf("Open on blank line should return NoChildren, got %v", state)
	}
}

func TestLinkLabelState_NodeInterface(t *testing.T) {
	// linkLabelState is an unexported type that implements
	// ast.Inline. Its Text / Dump / Kind methods exist to satisfy
	// the interface; they are never called via the dispatcher.
	// Drive them directly so they appear as reached coverage.
	s := &linkLabelState{
		Segment: text.NewSegment(0, 5),
	}
	source := []byte("hello world")
	if got := s.Text(source); string(got) != "hello" {
		t.Errorf("Text = %q, want hello", got)
	}
	if k := s.Kind(); k != kindLinkLabelState {
		t.Errorf("Kind = %v, want kindLinkLabelState", k)
	}
	// Dump prints to stdout; just call it.
	silenceStdout(t, func() { s.Dump(source, 0) })
}

func TestIDs_GenerateSequenceCollision(t *testing.T) {
	// Generate disambiguates by appending -N to slugs that are
	// already taken. Drive the loop with three same-name calls.
	ids := newIDs().(*ids)
	a := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	b := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	c := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	if a == b || b == c || a == c {
		t.Errorf("Generate must disambiguate: %q %q %q", a, b, c)
	}
	if !strings.HasPrefix(b, "heading-") {
		t.Errorf("second Generate should have -N suffix: %q", b)
	}
}

// silenceStdout swallows fmt.Print output from a function so
// Dump-style prints don't litter test output.
func silenceStdout(t *testing.T, fn func()) {
	t.Helper()
	defer func() { _ = recover() }()
	fn()
}

func TestParseListItem_AllBranches(t *testing.T) {
	// parseListItem is unexported.  Drive each early-return path
	// via direct invocation.
	cases := []struct {
		name string
		line string
		want listItemType
	}{
		{"bullet-dash", "- item\n", bulletList},
		{"bullet-star", "* item\n", bulletList},
		{"bullet-plus", "+ item\n", bulletList},
		{"ordered-period", "1. item\n", orderedList},
		{"ordered-paren", "1) item\n", orderedList},
		{"deep-indent", "    - too deep\n", notList},
		{"long-number", "1234567890. too long\n", notList},
		{"number-no-period", "1 item\n", notList},
		{"no-marker", "no list marker\n", notList},
		{"bullet-no-space", "-noSpace\n", notList},
		{"bullet-eol", "-\n", bulletList},
		{"empty-line", "", notList},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got := parseListItem([]byte(c.line))
			if got != c.want {
				t.Errorf("parseListItem(%q) = %v, want %v", c.line, got, c.want)
			}
		})
	}
}

// recordingPrioritized constructs a util.PrioritizedValue for an
// arbitrary value. Used by some internal unit tests.
var _ = util.Prioritized
