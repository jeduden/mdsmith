package listscan

import (
	"testing"
	"unsafe"
)

// TestStructLayout asserts the optimal field-aligned sizes for the hot-path
// structs in this package. Each struct is allocated in slices during Parse;
// tighter layouts reduce per-Check memory and improve cache utilisation.
// The test fails (red) until fields are reordered to achieve the target sizes.
func TestStructLayout(t *testing.T) {
	tests := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"Item", unsafe.Sizeof(Item{}), 32},
		{"List", unsafe.Sizeof(List{}), 64},
		{"frame", unsafe.Sizeof(frame{}), 48},
		{"markerInfo", unsafe.Sizeof(markerInfo{}), 24},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("unsafe.Sizeof(%s{}) = %d; want %d (reorder fields to eliminate padding)",
				tc.name, tc.got, tc.want)
		}
	}
}
