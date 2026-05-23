package parser_test

import (
	"testing"

	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
)

// Plan-197/198 contract: DefaultParagraphTransformers must return a
// FRESH linkReferenceParagraphTransformer instance per call so that
// every parser owns its own transformer (with its own reusable
// BlockReader) instead of sharing the upstream singleton.  Two
// consecutive calls must return distinct entries at priority 100.
//
// The assertion deliberately does NOT pin the slice length to 1:
// pkg/markdown/newPooledParser uses DefaultParagraphTransformers()
// as a building block and the upstream list may grow with future
// goldmark sync.  What matters is (a) the link-ref transformer is
// present at priority 100 and (b) repeated calls return distinct
// instances at that priority.
func TestDefaultParagraphTransformers_FreshPerCall(t *testing.T) {
	a := parser.DefaultParagraphTransformers()
	b := parser.DefaultParagraphTransformers()
	if len(a) < 1 {
		t.Fatalf("expected at least 1 default transformer, got %d", len(a))
	}
	if len(b) != len(a) {
		t.Fatalf("two calls returned different lengths: %d vs %d", len(a), len(b))
	}

	findP100 := func(list []util.PrioritizedValue) int {
		for i, e := range list {
			if e.Priority == 100 {
				return i
			}
		}
		return -1
	}
	ai := findP100(a)
	bi := findP100(b)
	if ai < 0 || bi < 0 {
		t.Fatalf("no priority-100 transformer found (a=%d, b=%d)", ai, bi)
	}
	if a[ai].Value == b[bi].Value {
		t.Error("DefaultParagraphTransformers must return a fresh priority-100 transformer instance per call")
	}
}
