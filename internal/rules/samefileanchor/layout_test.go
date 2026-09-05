package samefileanchor

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

// TestAnchorCheckerFieldOrder guards the GC ptrdata region of
// anchorChecker: built (a bool, no pointers) must come after the
// pointer-containing r, f, slugs, and diags fields. Go's GC ptrdata
// for a struct spans from offset 0 through the end of the last
// pointer-containing field, so a scalar declared before diags (the
// last pointer field) costs GC scan bytes for nothing — a saving of
// one word on a 64-bit platform. See
// docs/development/high-performance-go.md "Struct layout". The test
// fails (red) until built is moved after diags.
func TestAnchorCheckerFieldOrder(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(anchorChecker{}))
}
