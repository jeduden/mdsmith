// Package fieldorder checks that a struct's pointer-bearing fields
// are declared before its scalar fields, per
// docs/development/high-performance-go.md "Group pointer fields
// first, scalars last — GC ptrdata spans through the last pointer
// field". Shared between pkg/goldmark/ast and pkg/goldmark/parser so
// their hot per-token struct layout tests do not each carry their own
// copy of the same reflection logic.
package fieldorder

import "reflect"

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
