package crossfilereferenceintegrity

import (
	"testing"
	"unsafe"
)

// TestStructLayout asserts the optimal size for the Rule struct.
// Moving bool fields to the end (previously between larger fields, wasting
// padding bytes) reduces size from 128 to 120 bytes and improves cache
// utilisation across per-Check calls. The cachedGlobSettingsErr memo
// (globSettingsErr, globSettingsMu, globSettingsDone) added 24 bytes of
// data; grouped with the two trailing bools, that 14-byte tail block
// still rounds up to a 16-byte multiple of the struct's 8-byte
// alignment, so the total grows from 120 to 144 with no extra padding
// beyond that unavoidable 2-byte tail.
func TestStructLayout(t *testing.T) {
	got := unsafe.Sizeof(Rule{})
	const want = uintptr(144)
	if got != want {
		t.Errorf("unsafe.Sizeof(Rule{}) = %d; want %d (reorder fields to eliminate padding)",
			got, want)
	}
}
