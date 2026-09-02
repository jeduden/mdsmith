package uniquefrontmatter

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

// TestPathEntryFieldOrder guards the GC ptrdata region of pathEntry:
// line (an int, no pointers) must come after the pointer-containing
// value and firstPath strings. Go's GC ptrdata for a struct spans
// from offset 0 through the end of the last pointer-containing
// field, so a scalar sandwiched between two string fields costs GC
// scan bytes for nothing — a saving of one word on a 64-bit
// platform. pathEntry is stored once per violation in
// scopeIndex.byPath, an index built across the whole workspace, so
// the saving scales with corpus size. See
// docs/development/high-performance-go.md "Struct layout". The test
// fails (red) until line is moved after firstPath.
func TestPathEntryFieldOrder(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(pathEntry{}))
}
