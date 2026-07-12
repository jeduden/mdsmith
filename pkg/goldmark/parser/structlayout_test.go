package parser

// Pins the struct-layout guidance in
// docs/development/high-performance-go.md ("Group pointer fields
// first, scalars last — GC ptrdata spans through the last pointer
// field") for the parser package's hottest per-token structs:
// Delimiter (one per emphasis-run candidate in every file's inline
// parse), linkLabelState (one per bracket candidate), and fenceData
// (one per fenced-code-block open).

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/internal/fieldorder"
)

func TestDelimiter_PointerFieldsBeforeScalars(t *testing.T) {
	// Index 0 is the embedded ast.BaseInline; only Delimiter's own
	// declared fields are checked here.
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(Delimiter{}), 1)
}

func TestLinkLabelState_PointerFieldsBeforeScalars(t *testing.T) {
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(linkLabelState{}), 1)
}

func TestFenceData_PointerFieldsBeforeScalars(t *testing.T) {
	// fenceData has no embedded base struct, so every field is checked.
	fieldorder.AssertPointersBeforeScalars(t, reflect.TypeOf(fenceData{}), 0)
}
