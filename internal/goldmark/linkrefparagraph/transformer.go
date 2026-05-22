package linkrefparagraph

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Transformer is the link-reference paragraph transformer. Unlike
// goldmark's singleton, each instance owns a reusable *text.BlockReader
// that is Reset for every paragraph instead of freshly allocated. The
// transformer is NOT safe for concurrent use; mdsmith's parserPool
// (pkg/markdown/parser.go) hands one parser-instance-with-transformer
// per goroutine, so concurrency is delegated to that pool.
//
// On every Transform call:
//   - First call (or first call with a new source []byte): allocate
//     the BlockReader once. text.NewBlockReader has no SetSource, so
//     we re-allocate when the document source changes.
//   - Subsequent calls within the same parse: block.Reset(lines).
//
// In practice, mdsmith's parserPool sees source bytes that change
// per Parse() call (each File pass), so the BlockReader is allocated
// once per Parse and reused across every paragraph in that Parse.
type Transformer struct {
	block  text.BlockReader
	source []byte // identity check for cross-Parse source change
}

// New returns a fresh Transformer. Use one per parser.Parser
// instance, not as a global singleton.
func New() *Transformer {
	return &Transformer{}
}

// Transform is the paragraph-transformer entry point. It mirrors the
// goldmark linkReferenceParagraphTransformer.Transform body 1-for-1
// except for the BlockReader acquisition: we own one instance and
// Reset it per call, rather than allocate fresh.
func (t *Transformer) Transform(node *ast.Paragraph, reader text.Reader, pc parser.Context) {
	lines := node.Lines()
	src := reader.Source()
	if t.block == nil || !sameSlice(t.source, src) {
		t.block = text.NewBlockReader(src, lines)
		t.source = src
	} else {
		t.block.Reset(lines)
	}
	block := t.block
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

// sameSlice reports whether a and b refer to the same underlying byte
// array start (cheap pointer identity check without going through
// reflect.SliceHeader). When mdsmith's Parse hands new source bytes
// to a parser, we need to allocate a fresh BlockReader because
// text.BlockReader's source field is set at construction with no
// setter.
func sameSlice(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}
