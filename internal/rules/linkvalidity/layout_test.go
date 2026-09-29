package linkvalidity

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

// TestRevMatchFieldOrder guards the GC ptrdata region of revMatch:
// the pointer-containing text and url byte slices must precede the
// scalar col0 and matchEnd ints. Go's GC ptrdata for a struct spans
// from offset 0 through the last pointer-containing field's pointer
// word; with the scalars first (the prior order), ptrdata still had
// to cover both slices, so grouping the slices first instead shrinks
// ptrdata from 48 bytes to 32 on a 64-bit platform. See
// docs/development/high-performance-go.md "Struct layout". The test
// fails (red) until col0 and matchEnd are moved after text and url.
func TestRevMatchFieldOrder(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(revMatch{}))
}
