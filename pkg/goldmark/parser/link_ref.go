package parser

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// linkReferenceParagraphTransformer extracts link-reference
// definitions from a paragraph. Each instance owns a reusable
// text.BlockReader, re-Reset for every paragraph in a single
// parse — upstream goldmark's `text.NewBlockReader(reader.Source(),
// lines)` at the top of Transform was a per-paragraph allocation
// hot spot (plan-195 profile, ~13.6 % of corpus allocations). The
// fork shares one BlockReader across every paragraph the same
// parser sees, the way parser.go's inline pass already does for
// every block.
//
// Concurrency: a transformer carries mutable state (block, source)
// across Transform calls, so it is NOT safe to share between
// concurrent parsers. mdsmith's parserPool gives each Get caller
// exclusive access to one parser-with-transformer pair until Put.
// Callers must obtain a fresh transformer via
// NewLinkReferenceParagraphTransformer() — DefaultParagraphTransformers
// does this for every parser it builds.
type linkReferenceParagraphTransformer struct {
	block  text.BlockReader
	source []byte // identity check for cross-Parse source change
}

// LinkReferenceParagraphTransformer is retained for backwards
// compatibility with upstream goldmark's public API. Direct use
// across goroutines is unsafe; callers that need a transformer
// should call NewLinkReferenceParagraphTransformer() instead and
// pair one instance with one parser. DefaultParagraphTransformers
// already does this for the default parser path.
//
// Deprecated: use NewLinkReferenceParagraphTransformer for new code.
var LinkReferenceParagraphTransformer = &linkReferenceParagraphTransformer{}

// NewLinkReferenceParagraphTransformer returns a fresh
// linkReferenceParagraphTransformer. Each parser should hold its
// own instance; sharing across parsers (or goroutines) is unsafe.
func NewLinkReferenceParagraphTransformer() ParagraphTransformer {
	return &linkReferenceParagraphTransformer{}
}

// Reset drops references to the most recently parsed document's
// source bytes and BlockReader. Pool consumers that put the parent
// parser back into a pool must call Reset before Put, so the idle
// pool slot does not pin a large document buffer — or the
// most-recent Parse's arena via the BlockReader's SegmentsCreator
// back-pointer.
func (p *linkReferenceParagraphTransformer) Reset() {
	p.block = nil
	p.source = nil
}

func (p *linkReferenceParagraphTransformer) Transform(node *ast.Paragraph, reader text.Reader, pc Context) {
	lines := node.Lines()
	src := reader.Source()
	if p.block == nil || !sameByteSlice(p.source, src) {
		p.block = text.NewBlockReader(src, lines)
		p.source = src
	} else {
		p.block.Reset(lines)
	}
	// Wire the per-Parse arena into this internal BlockReader so
	// FindClosure here lands its Segments in arena memory like the
	// top-level reader. Re-set on every Transform because the
	// underlying BlockReader may be reused across parses (when the
	// source bytes are identical) but the arena rotates each Parse.
	setSegmentsCreator(p.block, ArenaForContext(pc))
	block := p.block
	removes := [][2]int{}
	for {
		ref, start, end := parseLinkReferenceDefinition(block, pc)
		if start > -1 {
			if start == 0 {
				ref.SetBlankPreviousLines(node.HasBlankPreviousLines())
			}
			node.Parent().InsertBefore(node.Parent(), node, ref)
			for i := start + 1; i < end; i++ {
				ref.Lines().Append(lines.At(i))
			}
			seg := ref.Lines().At(ref.Lines().Len() - 1)
			ref.Lines().Set(ref.Lines().Len()-1, seg.TrimRightSpace(reader.Source()))
			if start == end {
				end++
			}
			removes = append(removes, [2]int{start, end})
			continue
		}
		break
	}

	// Compact the paragraph by removing every line span that
	// became a LinkReferenceDefinition. parseLinkReferenceDefinition
	// only matches consecutive defs at the head of the paragraph
	// (the loop above stops the first time it returns -1), so the
	// removes slice is always a contiguous chain:
	//   removes[i+1][0] == removes[i][1]
	// Under that invariant `offset = remove[1]` (the absolute end
	// of the previously removed span) is equivalent to "number of
	// lines removed so far", so `remove[i+1][0] - offset` is
	// always 0 and the formula correctly indexes into the current
	// (already-compacted) line slice.
	offset := 0
	for _, remove := range removes {
		if lines.Len() == 0 {
			break
		}
		s := lines.Sliced(remove[1]-offset, lines.Len())
		lines.SetSliced(0, remove[0]-offset)
		lines.AppendAll(s)
		offset = remove[1]
	}

	if lines.Len() == 0 {
		node.Parent().RemoveChild(node.Parent(), node)
		return
	}

	node.SetLines(lines)
}

// sameByteSlice reports whether a and b are the same slice — both
// share the same backing-array start AND have the same length. Two
// slices into the same array but with different lengths represent
// different views (different document sources for the BlockReader
// reuse check), so we treat them as not the same.
//
// The BlockReader's source field is set at construction with no
// setter, so when the document source changes between parses on
// the same transformer we must allocate a fresh BlockReader rather
// than reuse the old one. Empty slices compare equal because there
// is no first-element pointer to inspect and reuse is harmless.
func sameByteSlice(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

func parseLinkReferenceDefinition(block text.Reader, pc Context) (ast.Node, int, int) {
	block.SkipSpaces()
	line, _ := block.PeekLine()
	if line == nil {
		return nil, -1, -1
	}
	startLine, _ := block.Position()
	width, pos := util.IndentWidth(line, 0)
	if width > 3 {
		return nil, -1, -1
	}
	if width != 0 {
		pos++
	}
	if line[pos] != '[' {
		return nil, -1, -1
	}
	_, startPos := block.Position()
	block.Advance(pos + 1)
	segments, found := block.FindClosure('[', ']', linkFindClosureOptions)
	if !found {
		return nil, -1, -1
	}
	var label []byte
	if segments.Len() == 1 {
		label = block.Value(segments.At(0))
	} else {
		for i := range segments.Len() {
			s := segments.At(i)
			label = append(label, block.Value(s)...)
		}
	}
	if util.IsBlank(label) {
		return nil, -1, -1
	}
	if block.Peek() != ':' {
		return nil, -1, -1
	}
	block.Advance(1)
	block.SkipSpaces()
	destination, ok := parseLinkDestination(block)
	if !ok {
		return nil, -1, -1
	}
	line, _ = block.PeekLine()
	isNewLine := line == nil || util.IsBlank(line)

	endLine, _ := block.Position()
	_, spaces, _ := block.SkipSpaces()
	opener := block.Peek()
	if opener != '"' && opener != '\'' && opener != '(' {
		if !isNewLine {
			return nil, -1, -1
		}
		ref := ast.NewLinkReferenceDefinition(label, destination, nil)
		ref.Lines().Append(startPos)
		pc.AddReference(newASTReference(ref))
		return ref, startLine, endLine + 1
	}
	if spaces == 0 {
		return nil, -1, -1
	}
	block.Advance(1)
	closer := opener
	if opener == '(' {
		closer = ')'
	}
	segments, found = block.FindClosure(opener, closer, linkFindClosureOptions)
	if !found {
		if !isNewLine {
			return nil, -1, -1
		}
		ref := ast.NewLinkReferenceDefinition(label, destination, nil)
		ref.Lines().Append(startPos)
		pc.AddReference(newASTReference(ref))
		block.AdvanceLine()
		return ref, startLine, endLine + 1
	}
	var title []byte
	if segments.Len() == 1 {
		title = block.Value(segments.At(0))
	} else {
		for i := range segments.Len() {
			s := segments.At(i)
			title = append(title, block.Value(s)...)
		}
	}

	line, _ = block.PeekLine()
	if line != nil && !util.IsBlank(line) {
		if !isNewLine {
			return nil, -1, -1
		}
		ref := ast.NewLinkReferenceDefinition(label, destination, title)
		ref.Lines().Append(startPos)
		pc.AddReference(newASTReference(ref))
		return ref, startLine, endLine
	}

	endLine, _ = block.Position()
	ref := ast.NewLinkReferenceDefinition(label, destination, title)
	ref.Lines().Append(startPos)
	pc.AddReference(newASTReference(ref))
	return ref, startLine, endLine + 1
}
