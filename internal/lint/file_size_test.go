package lint

import (
	"testing"
	"unsafe"
)

// fileSizeBudget pins File's struct size. File is allocated exactly
// once per NewFile call — the single most common allocation across
// the engine's per-file Check path, so shaving its size compounds
// across every file in the workspace. The four scalar fields
// (LineOffset, MaxInputBytes, StripFrontMatter, DryRun) previously
// sat at the top of the struct, interleaved with 8-byte-aligned
// pointer/slice/string fields; large-to-small ordering — pointer and
// slice fields first, the two 8-byte scalars next, the two
// byte-sized bools last — packs the struct into 640 bytes instead of
// 688 (docs/development/high-performance-go.md#struct-layout).
const fileSizeBudget = 640

func TestFile_SizeBudget(t *testing.T) {
	got := unsafe.Sizeof(File{})
	if got > fileSizeBudget {
		t.Fatalf("unsafe.Sizeof(File{}) = %d, budget = %d (field order wastes padding)",
			got, fileSizeBudget)
	}
}
