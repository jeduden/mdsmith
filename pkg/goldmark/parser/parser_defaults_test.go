package parser_test

import (
	"testing"

	"github.com/yuin/goldmark/parser"
)

// Plan-197/198 contract: DefaultParagraphTransformers must return a
// FRESH linkReferenceParagraphTransformer instance per call so that
// every parser owns its own transformer (with its own reusable
// BlockReader) instead of sharing the upstream singleton. Two
// consecutive calls must return distinct entries.
func TestDefaultParagraphTransformers_FreshPerCall(t *testing.T) {
	a := parser.DefaultParagraphTransformers()
	b := parser.DefaultParagraphTransformers()
	if len(a) != 1 {
		t.Fatalf("expected 1 default transformer, got %d", len(a))
	}
	if len(b) != 1 {
		t.Fatalf("expected 1 default transformer, got %d", len(b))
	}
	if a[0].Priority != 100 {
		t.Errorf("expected priority 100, got %d", a[0].Priority)
	}
	if a[0].Value == b[0].Value {
		t.Error("DefaultParagraphTransformers must return a fresh transformer instance per call")
	}
}
