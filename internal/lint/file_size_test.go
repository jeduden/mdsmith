package lint

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

// fileSizeBudget pins File's struct size. File is allocated exactly
// once per NewFile call — the single most common allocation across
// the engine's per-file Check path, so shaving its size compounds
// across every file in the workspace. The four scalar fields
// (LineOffset, MaxInputBytes, StripFrontMatter, DryRun) previously
// sat at the top of the struct, interleaved with 8-byte-aligned
// pointer/slice/string fields; grouping every pointer-bearing field
// first, then the two byte-sized bools (packed alongside the other
// 4-byte-aligned lazy-init guards), then the two 8-byte scalars last
// — packs the struct into 640 bytes instead of 688
// (docs/development/high-performance-go.md#struct-layout). 656 adds
// headingTextCache (map header) + headingTextCacheMu (sync.Mutex),
// both pointer-bearing-or-guard fields placed ahead of the trailing
// scalars per the same ordering rule. 656 crosses a Go allocator
// size-class boundary from 640's class (641-704 all round up to 704,
// measured) — see headingTextCache's own comment in file.go for why
// the two new fields were kept anyway.
const fileSizeBudget = 656

func TestFile_SizeBudget(t *testing.T) {
	got := unsafe.Sizeof(File{})
	if got > fileSizeBudget {
		t.Fatalf("unsafe.Sizeof(File{}) = %d, budget = %d (field order wastes padding)",
			got, fileSizeBudget)
	}
}

// TestFile_PointerFieldsPrecedeScalars guards the ordering half of
// the same layout: every pointer/slice/map/interface field (and the
// lazy-init guards, all pointer-free) must precede the four trailing
// scalars. TestFile_SizeBudget alone would not catch a future edit
// that keeps total size at 640 bytes while moving a pointer-bearing
// field below a scalar guard field.
func TestFile_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(File{}))
}
