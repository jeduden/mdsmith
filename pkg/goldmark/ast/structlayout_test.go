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

func TestBaseNode_PointerFieldsBeforeScalars(t *testing.T) {
	// BaseNode is embedded in every AST node of every parse, so any
	// wasted GC ptrdata here multiplies across the whole tree.
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(BaseNode{}), 0)
}
