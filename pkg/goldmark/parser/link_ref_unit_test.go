package parser

// Unit tests for the fork-specific additions in link_ref.go:
//   - NewLinkReferenceParagraphTransformer
//   - linkReferenceParagraphTransformer.Reset
//   - sameByteSlice (unexported)
//
// The behavioural Transform tests live in link_ref_behavior_test.go
// (external package) so they can drive the transformer through the
// public parser.Parser API.

import (
	"testing"

	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func TestNewLinkReferenceParagraphTransformer_ReturnsFreshInstance(t *testing.T) {
	a := NewLinkReferenceParagraphTransformer()
	b := NewLinkReferenceParagraphTransformer()
	if a == nil || b == nil {
		t.Fatal("constructor must return non-nil")
	}
	if a == b {
		t.Fatal("each call must return a distinct instance for per-parser ownership")
	}
}

func TestLinkRefTransformerReset_NilsBlockAndSource(t *testing.T) {
	tr := &linkReferenceParagraphTransformer{
		block:  text.NewBlockReader([]byte("doc"), nil),
		source: []byte("doc"),
	}
	tr.Reset()
	if tr.block != nil {
		t.Errorf("block must be nil after Reset, got %v", tr.block)
	}
	if tr.source != nil {
		t.Errorf("source must be nil after Reset, got %v", tr.source)
	}
}

func TestLinkRefTransformerReset_IdempotentOnZeroValue(t *testing.T) {
	tr := &linkReferenceParagraphTransformer{}
	tr.Reset() // must not panic on a never-used transformer
	if tr.block != nil || tr.source != nil {
		t.Error("Reset on zero value must leave fields nil")
	}
}

func TestSameByteSlice(t *testing.T) {
	a := []byte("hello world")
	b := a[:]
	if !sameByteSlice(a, b) {
		t.Error("aliased slices must compare equal")
	}
	if !sameByteSlice(nil, nil) {
		t.Error("nil/nil must compare equal (both empty)")
	}
	if !sameByteSlice([]byte{}, nil) {
		t.Error("empty and nil slices must compare equal")
	}
	if sameByteSlice(a, []byte("hi")) {
		t.Error("differing lengths must short-circuit to false")
	}
	c := append([]byte(nil), a...) // distinct backing array, equal content
	if sameByteSlice(a, c) {
		t.Error("distinct backing arrays with equal content must compare unequal")
	}
}

// TestLinkRefTransform_BlockReaderReusedAcrossParses asserts the
// reuse claim made by the transformer's doc string: parsing the
// same source bytes twice must leave the BlockReader instance
// untouched (the sameByteSlice path takes Reset, not realloc).
// The previous behavioural test only checked that link references
// landed in the parser context — it did not actually verify the
// `if p.block == nil || !sameByteSlice(...)` reuse branch.
func TestLinkRefTransform_BlockReaderReusedAcrossParses(t *testing.T) {
	tr := NewLinkReferenceParagraphTransformer().(*linkReferenceParagraphTransformer)
	p := NewParser(
		WithBlockParsers(DefaultBlockParsers()...),
		WithInlineParsers(DefaultInlineParsers()...),
		WithParagraphTransformers(util.Prioritized(tr, 1000)),
	)
	src := []byte("[a]: /1\n\nparagraph one\n\n[b]: /2\n\nparagraph two\n")

	p.Parse(text.NewReader(src), WithContext(NewContext()))
	firstBlock := tr.block
	if firstBlock == nil {
		t.Fatal("first parse must leave transformer block non-nil")
	}

	p.Parse(text.NewReader(src), WithContext(NewContext()))
	if tr.block != firstBlock {
		t.Error("second parse over the same source reallocated the BlockReader — Reset path was expected")
	}
}

// TestLinkRefTransform_BlockReaderReallocatedOnSourceChange asserts
// the complementary path: when the source bytes change identity
// between Parse calls, sameByteSlice returns false and the
// transformer must allocate a fresh BlockReader (the old one
// pinned different source bytes).
func TestLinkRefTransform_BlockReaderReallocatedOnSourceChange(t *testing.T) {
	tr := NewLinkReferenceParagraphTransformer().(*linkReferenceParagraphTransformer)
	p := NewParser(
		WithBlockParsers(DefaultBlockParsers()...),
		WithInlineParsers(DefaultInlineParsers()...),
		WithParagraphTransformers(util.Prioritized(tr, 1000)),
	)
	src1 := []byte("[a]: /1\n\nfirst doc\n")
	src2 := []byte("[b]: /2\n\nsecond doc\n")

	p.Parse(text.NewReader(src1), WithContext(NewContext()))
	block1 := tr.block
	p.Parse(text.NewReader(src2), WithContext(NewContext()))
	block2 := tr.block

	if block1 == nil || block2 == nil {
		t.Fatal("block must be non-nil after each parse")
	}
	if block1 == block2 {
		t.Error("source change should reallocate BlockReader (sameByteSlice returns false)")
	}
}

func TestSameByteSlice_Subslices(t *testing.T) {
	src := []byte("hello world")
	left := src[:5]   // "hello"
	right := src[:5]  // also "hello", same backing
	if !sameByteSlice(left, right) {
		t.Error("two subslices over the same backing with the same start must compare equal")
	}
	mid := src[6:] // "world", same backing but different start
	if sameByteSlice(left, mid[:5]) {
		// Both are length 5 over the same underlying array but start
		// at different offsets — sameByteSlice compares &slice[0], so
		// these must NOT be equal.
		t.Error("subslices that start at different offsets must compare unequal")
	}
}
