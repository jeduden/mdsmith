// Package fieldorder checks that a struct's pointer-bearing fields
// are declared before its scalar fields, per
// docs/development/high-performance-go.md "Group pointer fields
// first, scalars last — GC ptrdata spans through the last pointer
// field". Shared between pkg/goldmark/ast and pkg/goldmark/parser so
// their hot per-token struct layout tests do not each carry their own
// copy of the same reflection logic.
package fieldorder

import (
	"reflect"
	"testing"
)

// IsPointerish reports whether a field's kind carries at least one
// pointer word that the GC must scan (a pointer, interface, slice,
// map, chan, func, string header, or unsafe.Pointer).
func IsPointerish(k reflect.Kind) bool {
	switch k {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map,
		reflect.Chan, reflect.Func, reflect.String, reflect.UnsafePointer:
		return true
	}
	return false
}

// AssertPointersBeforeScalars checks that, among typ's fields
// starting at index skip (use 1 to skip an embedded base struct), no
// scalar (non-pointerish) field is followed by a pointerish one. A
// pointerish field declared after a scalar field extends the struct's
// GC ptrdata past that scalar for no reason.
func AssertPointersBeforeScalars(t testing.TB, typ reflect.Type, skip int) {
	t.Helper()
	seenScalar := false
	for i := skip; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if IsPointerish(f.Type.Kind()) {
			if seenScalar {
				t.Errorf("field %s (%s, pointer-ish) is declared after a scalar field; "+
					"pointer fields should precede scalars to shrink GC ptrdata",
					f.Name, f.Type)
			}
		} else {
			seenScalar = true
		}
	}
}
