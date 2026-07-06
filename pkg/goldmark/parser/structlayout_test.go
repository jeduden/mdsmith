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

	"github.com/stretchr/testify/assert"
)

// isPointerish reports whether a field's kind carries at least one
// pointer word that the GC must scan (a pointer, interface, slice,
// map, chan, func, string header, or unsafe.Pointer).
func isPointerish(k reflect.Kind) bool {
	switch k {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map,
		reflect.Chan, reflect.Func, reflect.String, reflect.UnsafePointer:
		return true
	}
	return false
}

// assertOwnFieldsPointersBeforeScalars checks that, among typ's
// fields starting at index skip (use 1 to skip an embedded base
// struct), no scalar (non-pointerish) field is followed by a
// pointerish one. A pointerish field declared after a scalar field
// extends the struct's GC ptrdata past that scalar for no reason.
func assertOwnFieldsPointersBeforeScalars(t *testing.T, typ reflect.Type, skip int) {
	t.Helper()
	seenScalar := false
	for i := skip; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if isPointerish(f.Type.Kind()) {
			assert.Falsef(t, seenScalar,
				"field %s (%s, pointer-ish) is declared after a scalar field; "+
					"pointer fields should precede scalars to shrink GC ptrdata",
				f.Name, f.Type)
		} else {
			seenScalar = true
		}
	}
}

func TestDelimiter_PointerFieldsBeforeScalars(t *testing.T) {
	// Index 0 is the embedded ast.BaseInline; only Delimiter's own
	// declared fields are checked here.
	assertOwnFieldsPointersBeforeScalars(t, reflect.TypeOf(Delimiter{}), 1)
}

func TestLinkLabelState_PointerFieldsBeforeScalars(t *testing.T) {
	assertOwnFieldsPointersBeforeScalars(t, reflect.TypeOf(linkLabelState{}), 1)
}

func TestFenceData_PointerFieldsBeforeScalars(t *testing.T) {
	// fenceData has no embedded base struct, so every field is checked.
	assertOwnFieldsPointersBeforeScalars(t, reflect.TypeOf(fenceData{}), 0)
}
