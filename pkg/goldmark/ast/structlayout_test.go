package ast

// Pins the struct-layout guidance in
// docs/development/high-performance-go.md ("Group pointer fields
// first, scalars last — GC ptrdata spans through the last pointer
// field") for AutoLink, allocated once per autolink node parsed.

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
// pointerish one.
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

func TestAutoLink_PointerFieldsBeforeScalars(t *testing.T) {
	// Index 0 is the embedded BaseInline; only AutoLink's own
	// declared fields are checked here.
	assertOwnFieldsPointersBeforeScalars(t, reflect.TypeOf(AutoLink{}), 1)
}
