package index

import (
	"reflect"
	"testing"
)

// isPointerish reports whether t's GC ptrdata bitmap must mark this
// field's word(s), directly or via a nested type.
func isPointerish(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.String, reflect.Slice, reflect.Map,
		reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer:
		return true
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if isPointerish(t.Field(i).Type) {
				return true
			}
		}
		return false
	case reflect.Array:
		return t.Len() > 0 && isPointerish(t.Elem())
	default:
		return false
	}
}

// assertPointerFieldsFirst fails if a pointer-containing field is
// declared after a scalar-only field. Go's GC ptrdata for a struct
// spans from offset 0 through the end of the last pointer-containing
// field; a scalar sandwiched before that point is scanned for nothing,
// and a scalar declared after every pointer field costs nothing. See
// docs/development/high-performance-go.md "Struct layout".
func assertPointerFieldsFirst(t *testing.T, typ reflect.Type) {
	t.Helper()
	sawScalar := false
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if isPointerish(f.Type) {
			if sawScalar {
				t.Fatalf("%s.%s (pointer-containing) is declared after a scalar field; "+
					"reorder so all pointer fields precede scalar fields to minimize GC ptrdata",
					typ.Name(), f.Name)
			}
			continue
		}
		sawScalar = true
	}
}

func TestEdge_PointerFieldsPrecedeScalars(t *testing.T) {
	assertPointerFieldsFirst(t, reflect.TypeOf(Edge{}))
}

func TestSymbol_PointerFieldsPrecedeScalars(t *testing.T) {
	assertPointerFieldsFirst(t, reflect.TypeOf(Symbol{}))
}
