package parser

import (
	"bytes"
	"regexp"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

var allowedBlockTags = map[string]struct{}{
	"address":    {},
	"article":    {},
	"aside":      {},
	"base":       {},
	"basefont":   {},
	"blockquote": {},
	"body":       {},
	"caption":    {},
	"center":     {},
	"col":        {},
	"colgroup":   {},
	"dd":         {},
	"details":    {},
	"dialog":     {},
	"dir":        {},
	"div":        {},
	"dl":         {},
	"dt":         {},
	"fieldset":   {},
	"figcaption": {},
	"figure":     {},
	"footer":     {},
	"form":       {},
	"frame":      {},
	"frameset":   {},
	"h1":         {},
	"h2":         {},
	"h3":         {},
	"h4":         {},
	"h5":         {},
	"h6":         {},
	"head":       {},
	"header":     {},
	"hr":         {},
	"html":       {},
	"iframe":     {},
	"legend":     {},
	"li":         {},
	"link":       {},
	"main":       {},
	"menu":       {},
	"menuitem":   {},
	"meta":       {},
	"nav":        {},
	"noframes":   {},
	"ol":         {},
	"optgroup":   {},
	"option":     {},
	"p":          {},
	"param":      {},
	"search":     {},
	"section":    {},
	"summary":    {},
	"table":      {},
	"tbody":      {},
	"td":         {},
	"tfoot":      {},
	"th":         {},
	"thead":      {},
	"title":      {},
	"tr":         {},
	"track":      {},
	"ul":         {},
}

// tagBuf is a fixed-capacity stack buffer for an ASCII tag name, sized so
// the longest HTML tag (and longer non-tags, truncated harmlessly) fits
// without a heap allocation.
type tagBuf struct {
	buf [32]byte
	n   int
}

// lowerInto copies the ASCII-lowercased bytes of b into the stack buffer,
// truncating anything past its capacity (a name that long is not in
// allowedBlockTags or the raw-text set, so truncation cannot cause a
// false match against the short tag names they contain).
func (t *tagBuf) lowerInto(b []byte) []byte {
	t.n = 0
	for _, c := range b {
		if t.n >= len(t.buf) {
			break
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		t.buf[t.n] = c
		t.n++
	}
	return t.buf[:t.n]
}

// tagInAllowedSet reports whether b is (case-insensitively) one of the
// type-6 HTML block tags, lowercasing into a stack buffer so the lookup
// allocates nothing — strings.ToLower(string(b)) allocated a new string
// on every trigger candidate line (docs/development/high-performance-go.md).
func tagInAllowedSet(b []byte) bool {
	var t tagBuf
	_, ok := allowedBlockTags[string(t.lowerInto(b))]
	return ok
}

// isRawTextTag reports whether b is (case-insensitively) one of the
// raw-text tags that type-7 openers defer to type 1 for.
func isRawTextTag(b []byte) bool {
	var t tagBuf
	switch string(t.lowerInto(b)) {
	case "script", "style", "pre":
		return true
	}
	return false
}

var htmlBlockType1OpenRegexp = regexp.MustCompile(`(?i)^[ ]{0,3}<(script|pre|style|textarea)(?:\s.*|>.*|/>.*|)(?:\r\n|\n)?$`) //nolint:golint,lll
var htmlBlockType1CloseRegexp = regexp.MustCompile(`(?i)^.*</(?:script|pre|style|textarea)>.*`)

var htmlBlockType2OpenRegexp = regexp.MustCompile(`^[ ]{0,3}<!\-\-`)
var htmlBlockType2Close = []byte{'-', '-', '>'}

var htmlBlockType3OpenRegexp = regexp.MustCompile(`^[ ]{0,3}<\?`)
var htmlBlockType3Close = []byte{'?', '>'}

var htmlBlockType4OpenRegexp = regexp.MustCompile(`^[ ]{0,3}<![A-Z]+.*(?:\r\n|\n)?$`)
var htmlBlockType4Close = []byte{'>'}

var htmlBlockType5OpenRegexp = regexp.MustCompile(`^[ ]{0,3}<\!\[CDATA\[`)
var htmlBlockType5Close = []byte{']', ']', '>'}

var htmlBlockType6Regexp = regexp.MustCompile(`^[ ]{0,3}<(?:/[ ]*)?([a-zA-Z]+[a-zA-Z0-9\-]*)(?:[ ].*|>.*|/>.*|)(?:\r\n|\n)?$`) //nolint:golint,lll

var htmlBlockType7Regexp = regexp.MustCompile(`^[ ]{0,3}<(/[ ]*)?([a-zA-Z]+[a-zA-Z0-9\-]*)(` + attributePattern + `*)[ ]*(?:>|/>)[ ]*(?:\r\n|\n)?$`) //nolint:golint,lll

type htmlBlockParser struct {
}

var defaultHTMLBlockParser = &htmlBlockParser{}

// NewHTMLBlockParser return a new BlockParser that can parse html
// blocks.
func NewHTMLBlockParser() BlockParser {
	return defaultHTMLBlockParser
}

func (b *htmlBlockParser) Trigger() []byte {
	return []byte{'<'}
}

func (b *htmlBlockParser) Open(parent ast.Node, reader text.Reader, pc Context) (ast.Node, State) {
	var node *ast.HTMLBlock
	line, segment := reader.PeekLine()
	last := pc.LastOpenedBlock().Node

	if m := htmlBlockType1OpenRegexp.FindSubmatchIndex(line); m != nil {
		node = ast.NewHTMLBlock(ast.HTMLBlockType1)
	} else if htmlBlockType2OpenRegexp.Match(line) {
		node = ast.NewHTMLBlock(ast.HTMLBlockType2)
	} else if htmlBlockType3OpenRegexp.Match(line) {
		node = ast.NewHTMLBlock(ast.HTMLBlockType3)
	} else if htmlBlockType4OpenRegexp.Match(line) {
		node = ast.NewHTMLBlock(ast.HTMLBlockType4)
	} else if htmlBlockType5OpenRegexp.Match(line) {
		node = ast.NewHTMLBlock(ast.HTMLBlockType5)
	} else if match := htmlBlockType7Regexp.FindSubmatchIndex(line); match != nil {
		isCloseTag := match[2] > -1 && bytes.Equal(line[match[2]:match[3]], []byte("/"))
		hasAttr := match[6] != match[7]
		tagBytes := line[match[4]:match[5]]
		if tagInAllowedSet(tagBytes) {
			node = ast.NewHTMLBlock(ast.HTMLBlockType6)
		} else if !isRawTextTag(tagBytes) &&
			!ast.IsParagraph(last) && !(isCloseTag && hasAttr) { // type 7 can not interrupt paragraph
			node = ast.NewHTMLBlock(ast.HTMLBlockType7)
		}
	}
	if node == nil {
		if match := htmlBlockType6Regexp.FindSubmatchIndex(line); match != nil {
			if tagInAllowedSet(line[match[2]:match[3]]) {
				node = ast.NewHTMLBlock(ast.HTMLBlockType6)
			}
		}
	}
	if node != nil {
		reader.AdvanceToEOL()
		node.Lines().Append(segment)
		return node, NoChildren
	}
	return nil, NoChildren
}

func (b *htmlBlockParser) Continue(node ast.Node, reader text.Reader, pc Context) State {
	htmlBlock := node.(*ast.HTMLBlock)
	lines := htmlBlock.Lines()
	line, segment := reader.PeekLine()
	var closurePattern []byte

	switch htmlBlock.HTMLBlockType {
	case ast.HTMLBlockType1:
		if lines.Len() == 1 {
			firstLine := lines.At(0)
			if htmlBlockType1CloseRegexp.Match(firstLine.Value(reader.Source())) {
				return Close
			}
		}
		if htmlBlockType1CloseRegexp.Match(line) {
			htmlBlock.ClosureLine = segment
			reader.AdvanceToEOL()
			return Close
		}
	case ast.HTMLBlockType2:
		closurePattern = htmlBlockType2Close
		fallthrough
	case ast.HTMLBlockType3:
		if closurePattern == nil {
			closurePattern = htmlBlockType3Close
		}
		fallthrough
	case ast.HTMLBlockType4:
		if closurePattern == nil {
			closurePattern = htmlBlockType4Close
		}
		fallthrough
	case ast.HTMLBlockType5:
		if closurePattern == nil {
			closurePattern = htmlBlockType5Close
		}

		if lines.Len() == 1 {
			firstLine := lines.At(0)
			if bytes.Contains(firstLine.Value(reader.Source()), closurePattern) {
				return Close
			}
		}
		if bytes.Contains(line, closurePattern) {
			htmlBlock.ClosureLine = segment
			reader.AdvanceToEOL()
			return Close
		}

	case ast.HTMLBlockType6, ast.HTMLBlockType7:
		if util.IsBlank(line) {
			return Close
		}
	}
	node.Lines().Append(segment)
	reader.AdvanceToEOL()
	return Continue | NoChildren
}

func (b *htmlBlockParser) Close(node ast.Node, reader text.Reader, pc Context) {
	// nothing to do
}

func (b *htmlBlockParser) CanInterruptParagraph() bool {
	return true
}

func (b *htmlBlockParser) CanAcceptIndentedLine() bool {
	return false
}
