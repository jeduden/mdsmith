package ast

// Pins the struct-layout guidance in
// docs/development/high-performance-go.md ("Group pointer fields
// first, scalars last — GC ptrdata spans through the last pointer
// field") for BaseNode itself. BaseNode is embedded in BaseBlock and
// BaseInline, which in turn are embedded in every concrete AST node
// type (Text, Heading, Paragraph, List, Link, ...), so it is the
// highest-multiplicity struct in the parser: one instance per node of
// every parse. Other *_test.go files in this package check only the
// fields concrete types declare on top of the embedded base (skip: 1);
// none of them checks the base struct's own layout.

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/internal/fieldorder"
)

func TestBaseNode_PointerFieldsBeforeScalars(t *testing.T) {
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(BaseNode{}), 0)
}
