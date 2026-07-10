package propernames

import (
	"testing"
	"unsafe"
)

// wrongMatchPtrdataBudget pins the GC ptrdata region of wrongMatch: the
// byte offset one word past its last pointer-bearing field. name's data
// pointer is the struct's only pointer word; placing name last pushes
// that word to offset 16 (ptrdata 24, per the fieldalignment analyzer:
// "struct with 24 pointer bytes could be 8"), forcing the GC to walk two
// extra non-pointer words in the per-word bitmap. wrongMatch is built
// once per wrong-cased occurrence findWrongMatches reports, so this adds
// up under MDS050 on prose-heavy files
// (docs/development/high-performance-go.md#struct-layout).
const wrongMatchPtrdataBudget = unsafe.Sizeof(uintptr(0))

func TestWrongMatch_PtrdataBudget(t *testing.T) {
	ptrdata := unsafe.Offsetof(wrongMatch{}.name) + unsafe.Sizeof(uintptr(0))
	if ptrdata > wrongMatchPtrdataBudget {
		t.Fatalf("wrongMatch ptrdata = %d, budget = %d (name is not the first field)",
			ptrdata, wrongMatchPtrdataBudget)
	}
}
