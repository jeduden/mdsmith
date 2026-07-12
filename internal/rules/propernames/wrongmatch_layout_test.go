package propernames

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

// TestWrongMatch_PointerFieldsPrecedeScalars guards the GC ptrdata region
// of wrongMatch: name (its only pointer-bearing field) must precede the
// scalar start/length fields, per docs/development/high-performance-go.md
// #struct-layout. wrongMatch is built once per wrong-cased occurrence
// findWrongMatches reports, so this adds up under MDS050 on prose-heavy
// files.
func TestWrongMatch_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(wrongMatch{}))
}
