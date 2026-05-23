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
