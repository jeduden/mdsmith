package ast

// Pins the struct-layout guidance in
// docs/development/high-performance-go.md ("Group pointer fields
// first, scalars last — GC ptrdata spans through the last pointer
// field") for AutoLink, allocated once per autolink node parsed.

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/internal/fieldorder"
)

func TestAutoLink_PointerFieldsBeforeScalars(t *testing.T) {
	// Index 0 is the embedded BaseInline; only AutoLink's own
	// declared fields are checked here.
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(AutoLink{}), 1)
}

// TestBaseNode_PointerFieldsBeforeScalars pins BaseNode's own field
// order. BaseNode is embedded in every AST node type (Document,
// Heading, Paragraph, Text, List, Link, Image, ...), so it is
// instantiated once per node on every parse of every file — the
// hottest struct in the codebase.
func TestBaseNode_PointerFieldsBeforeScalars(t *testing.T) {
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(BaseNode{}), 0)
}
