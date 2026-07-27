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

// TestBaseNode_PointerFieldsBeforeScalars pins BaseNode's layout.
// BaseNode is embedded in every AST node (Document, Heading,
// Paragraph, Text, Emphasis, Link, …), so it is the single
// most-allocated struct in the parser.
func TestBaseNode_PointerFieldsBeforeScalars(t *testing.T) {
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(BaseNode{}), 0)
}
