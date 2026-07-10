// Package structlayout asserts that a struct type orders its
// pointer-containing fields before its scalar fields. It is intended
// for use only from *_test.go files.
package structlayout

import "reflect"

// reporter is the subset of *testing.T this package needs. Depending
// on an interface instead of *testing.T directly lets this package's
// own tests exercise the failure path with a fake, without pulling
// in "testing" as a dependency of this non-_test.go file.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

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

// AssertPointerFieldsFirst reports every pointer-containing field
// declared after a scalar-only field. Go's GC ptrdata for a struct
// spans from offset 0 through the end of the last pointer-containing
// field; a scalar sandwiched before that point is scanned for
// nothing, and a scalar declared after every pointer field costs
// nothing. See docs/development/high-performance-go.md "Struct
// layout".
func AssertPointerFieldsFirst(t reporter, typ reflect.Type) {
	t.Helper()
	sawScalar := false
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if isPointerish(f.Type) {
			if sawScalar {
				t.Errorf("%s.%s (pointer-containing) is declared after a scalar field; "+
					"reorder so all pointer fields precede scalar fields to minimize GC ptrdata",
					typ.Name(), f.Name)
			}
			continue
		}
		sawScalar = true
	}
}
