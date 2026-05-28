package ext

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// AbbreviationDefinition is the AST node produced by the
// abbreviation block parser for a `*[term]: expansion` line. The
// node is raw so its content is not re-parsed as inline markup.
type AbbreviationDefinition struct {
	ast.BaseBlock
	Term      []byte
	Expansion []byte
}

// KindAbbreviationDefinition is the NodeKind of AbbreviationDefinition.
var KindAbbreviationDefinition = ast.NewNodeKind("AbbreviationDefinition")

// Kind implements ast.Node.
func (n *AbbreviationDefinition) Kind() ast.NodeKind { return KindAbbreviationDefinition }

// IsRaw implements ast.Node.
func (n *AbbreviationDefinition) IsRaw() bool { return true }

// Dump implements ast.Node.
func (n *AbbreviationDefinition) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Term":      string(n.Term),
		"Expansion": string(n.Expansion),
	}, nil)
}

// AbbreviationReference marks an inline occurrence of a defined
// abbreviation term. The referenced term text lives in the child
// Text node.
type AbbreviationReference struct {
	ast.BaseInline
	Term []byte
}

// KindAbbreviationReference is the NodeKind of AbbreviationReference.
var KindAbbreviationReference = ast.NewNodeKind("AbbreviationReference")

// Kind implements ast.Node.
func (n *AbbreviationReference) Kind() ast.NodeKind { return KindAbbreviationReference }

// Dump implements ast.Node.
func (n *AbbreviationReference) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Term": string(n.Term),
	}, nil)
}

// abbrTableKey is the parser-context key under which the
// abbreviation-definition table is stored during a single parse so
// the transformer can look it up after all blocks have been parsed.
var abbrTableKey = parser.NewContextKey()

// abbrTable is the in-parse table of defined abbreviations. Keys are
// canonical term byte sequences; values are expansion bytes.
type abbrTable map[string][]byte

// getAbbrTable returns the abbreviation table from the context,
// creating it on first access.
func getAbbrTable(pc parser.Context) abbrTable {
	if v := pc.Get(abbrTableKey); v != nil {
		return v.(abbrTable)
	}
	t := abbrTable{}
	pc.Set(abbrTableKey, t)
	return t
}

// --- block parser -----------------------------------------------------

// abbrDefPrefix is the literal `*[` that starts every definition.
var abbrDefPrefix = []byte("*[")

// abbreviationBlockParser parses `*[term]: expansion` lines as
// block-level AbbreviationDefinition nodes and records the term in
// the parse context so the transformer can mark references.
type abbreviationBlockParser struct{}

// Trigger implements parser.BlockParser.
func (p *abbreviationBlockParser) Trigger() []byte { return []byte{'*'} }

// Open implements parser.BlockParser.
//
// A definition line has the shape `*[TERM]: EXPANSION` with no
// leading indent beyond three spaces. TERM must be non-empty and
// may not contain a literal `]`. EXPANSION may be empty.
func (p *abbreviationBlockParser) Open(
	parent ast.Node, reader text.Reader, pc parser.Context,
) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	if line == nil {
		return nil, parser.NoChildren
	}
	trimmed := bytes.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	if indent >= 4 {
		return nil, parser.NoChildren
	}
	if !bytes.HasPrefix(trimmed, abbrDefPrefix) {
		return nil, parser.NoChildren
	}
	rest := trimmed[len(abbrDefPrefix):]
	rbracket := bytes.IndexByte(rest, ']')
	if rbracket <= 0 {
		return nil, parser.NoChildren
	}
	term := rest[:rbracket]
	afterBracket := rest[rbracket+1:]
	if len(afterBracket) == 0 || afterBracket[0] != ':' {
		return nil, parser.NoChildren
	}
	expansion := bytes.TrimSpace(afterBracket[1:])
	// Copy byte slices — the reader's line buffer is reused between
	// calls, so holding references to it would be unsafe.
	termCopy := append([]byte(nil), term...)
	expCopy := append([]byte(nil), expansion...)

	node := &AbbreviationDefinition{Term: termCopy, Expansion: expCopy}
	// Record the raw source span so detectors (and a future Fix) can
	// locate this definition without re-scanning.
	_, seg := reader.PeekLine()
	node.Lines().Append(seg)
	tbl := getAbbrTable(pc)
	tbl[string(termCopy)] = expCopy

	reader.AdvanceToEOL()
	return node, parser.NoChildren
}

// Continue implements parser.BlockParser.
//
// Definitions are always single-line; the parser closes immediately
// after Open consumes the line.
func (p *abbreviationBlockParser) Continue(
	n ast.Node, reader text.Reader, pc parser.Context,
) parser.State {
	return parser.Close
}

// Close implements parser.BlockParser.
func (p *abbreviationBlockParser) Close(n ast.Node, reader text.Reader, pc parser.Context) {}

// CanInterruptParagraph implements parser.BlockParser.
func (p *abbreviationBlockParser) CanInterruptParagraph() bool { return false }

// CanAcceptIndentedLine implements parser.BlockParser.
func (p *abbreviationBlockParser) CanAcceptIndentedLine() bool { return false }

// --- AST transformer --------------------------------------------------

// abbreviationTransformer runs after block parsing and rewrites
// inline Text nodes to mark whole-word occurrences of defined terms
// as AbbreviationReference nodes.
type abbreviationTransformer struct{}

// Transform implements parser.ASTTransformer.
func (t *abbreviationTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	raw := pc.Get(abbrTableKey)
	if raw == nil {
		return
	}
	table := raw.(abbrTable)
	if len(table) == 0 {
		return
	}
	source := reader.Source()

	// Walk every Text descendant in the document and rewrite
	// whole-word term matches. Code spans, fenced and indented code
	// blocks, and AbbreviationDefinition nodes are skipped so their
	// contents stay literal; everything else (paragraphs, headings,
	// list items, etc.) is eligible for marking.
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.CodeSpan, *ast.FencedCodeBlock, *ast.CodeBlock,
			*AbbreviationDefinition:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			rewriteText(node, table, source)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
}

// abbrMatch is an internal record of one abbreviation hit inside a
// Text node body. Offsets are relative to the body, not to f.Source.
type abbrMatch struct {
	start, end int
	term       string
}

// rewriteText splits a Text node around whole-word term matches and
// inserts AbbreviationReference nodes in their place. The original
// Text node's segment is shrunk to the prefix before the first
// match; subsequent content is appended as sibling nodes.
func rewriteText(t *ast.Text, table abbrTable, source []byte) {
	seg := t.Segment
	body := seg.Value(source)
	if len(body) == 0 {
		return
	}
	matches := findMatches(body, table)
	if len(matches) == 0 {
		return
	}
	parent := t.Parent()
	if parent == nil {
		return
	}
	applyMatches(parent, t, seg, body, matches)
}

// findMatches scans body for the longest whole-word match of any
// defined term at each word-boundary position and advances past each
// hit so occurrences never overlap.
func findMatches(body []byte, table abbrTable) []abbrMatch {
	var matches []abbrMatch
	for i := 0; i < len(body); {
		if i > 0 && isWordByte(body[i-1]) {
			i++
			continue
		}
		m, ok := bestMatchAt(body, i, table)
		if !ok {
			i++
			continue
		}
		matches = append(matches, m)
		i = m.end
	}
	return matches
}

// bestMatchAt returns the longest term in table that matches body
// starting at i, requiring a word boundary after the term.
func bestMatchAt(body []byte, i int, table abbrTable) (abbrMatch, bool) {
	best := abbrMatch{start: -1}
	for term := range table {
		tb := []byte(term)
		if !bytes.HasPrefix(body[i:], tb) {
			continue
		}
		endIdx := i + len(tb)
		if endIdx < len(body) && isWordByte(body[endIdx]) {
			continue
		}
		if len(tb) > best.end-best.start {
			best = abbrMatch{start: i, end: endIdx, term: term}
		}
	}
	if best.start < 0 {
		return abbrMatch{}, false
	}
	return best, true
}

// applyMatches rewrites the AST around t with the given matches.
// The original Text is either shrunk to the prefix (common case)
// or replaced entirely when the first match starts at offset 0.
func applyMatches(parent ast.Node, t *ast.Text, seg text.Segment, body []byte, matches []abbrMatch) {
	first := matches[0]
	var anchor ast.Node
	if first.start == 0 {
		ref := buildReference(seg, first)
		parent.ReplaceChild(parent, t, ref)
		anchor = ref
	} else {
		t.Segment = seg.WithStop(seg.Start + first.start)
		ref := buildReference(seg, first)
		parent.InsertAfter(parent, t, ref)
		anchor = ref
	}
	anchor = appendRestAfter(parent, anchor, seg, matches[1:], first.end)
	_ = appendTail(parent, anchor, seg, body, lastEnd(first, matches[1:]))
}

// appendRestAfter inserts gap-text and reference nodes for each
// remaining match after anchor, returning the last inserted node.
func appendRestAfter(parent, anchor ast.Node, seg text.Segment, rest []abbrMatch, prev int) ast.Node {
	for _, m := range rest {
		if m.start > prev {
			gap := ast.NewTextSegment(subSeg(seg, prev, m.start))
			parent.InsertAfter(parent, anchor, gap)
			anchor = gap
		}
		ref := buildReference(seg, m)
		parent.InsertAfter(parent, anchor, ref)
		anchor = ref
		prev = m.end
	}
	return anchor
}

// appendTail inserts the final Text node for body content after the
// last match, returning the inserted node (or anchor when no tail).
func appendTail(parent, anchor ast.Node, seg text.Segment, body []byte, prev int) ast.Node {
	if prev >= len(body) {
		return anchor
	}
	tail := ast.NewTextSegment(seg.WithStart(seg.Start + prev))
	parent.InsertAfter(parent, anchor, tail)
	return tail
}

// lastEnd returns the end offset of the final match in first+rest.
func lastEnd(first abbrMatch, rest []abbrMatch) int {
	if len(rest) == 0 {
		return first.end
	}
	return rest[len(rest)-1].end
}

// buildReference constructs an AbbreviationReference whose child
// Text covers the term's byte span.
func buildReference(parentSeg text.Segment, m abbrMatch) ast.Node {
	ref := &AbbreviationReference{Term: []byte(m.term)}
	ref.AppendChild(ref, ast.NewTextSegment(subSeg(parentSeg, m.start, m.end)))
	return ref
}

// isWordByte reports whether b is part of a word for abbreviation
// boundary purposes: ASCII alphanumeric or underscore.
func isWordByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// subSeg returns a new Segment that starts at parent.Start + start
// and stops at parent.Start + stop. Avoids chained WithStart /
// WithStop calls, which cannot be inlined because both are pointer
// methods on text.Segment.
func subSeg(parent text.Segment, start, stop int) text.Segment {
	s := parent.WithStart(parent.Start + start)
	return s.WithStop(parent.Start + stop)
}

// --- extender ---------------------------------------------------------

type abbreviationExt struct{}

// Abbreviation is the goldmark Extender that installs the block
// parser (priority 900 — later than most block parsers so it runs
// only when nothing else has claimed the line) and the AST
// transformer (priority 800 — runs after all blocks are finalized).
var Abbreviation goldmark.Extender = &abbreviationExt{}

// Extend implements goldmark.Extender.
func (e *abbreviationExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&abbreviationBlockParser{}, 900),
		),
		parser.WithASTTransformers(
			util.Prioritized(&abbreviationTransformer{}, 800),
		),
	)
}
