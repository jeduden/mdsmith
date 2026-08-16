package recipesafety

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBodyTemplateReserved_IsZeroByteSet pins
// docs/development/high-performance-go.md's "map[K]struct{} for sets —
// zero-byte value type": bodyTemplateReserved is used only as a pure
// membership set (its one call site is a truth-test, never a stored
// false), so its value type must be the zero-byte struct{}, not bool.
func TestBodyTemplateReserved_IsZeroByteSet(t *testing.T) {
	elem := reflect.TypeOf(bodyTemplateReserved).Elem()
	assert.Equalf(t, reflect.Struct, elem.Kind(),
		"bodyTemplateReserved value type is %s, want struct{}", elem.Kind())
	assert.Equalf(t, uintptr(0), elem.Size(),
		"bodyTemplateReserved value type is %d bytes, want the zero-byte struct{}", elem.Size())
}
