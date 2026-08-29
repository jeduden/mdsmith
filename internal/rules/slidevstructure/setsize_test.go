package slidevstructure

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertZeroByteSet pins docs/development/high-performance-go.md's
// "map[K]struct{} for sets — zero-byte value type": m is used only as
// a pure membership set (every call site is a truth-test, never a
// stored false), so its value type must be the zero-byte struct{},
// not bool.
func assertZeroByteSet(t *testing.T, name string, m any) {
	t.Helper()
	elem := reflect.TypeOf(m).Elem()
	assert.Equalf(t, reflect.Struct, elem.Kind(),
		"%s value type is %s, want struct{}", name, elem.Kind())
	assert.Equalf(t, uintptr(0), elem.Size(),
		"%s value type is %d bytes, want the zero-byte struct{}", name, elem.Size())
}

func TestBuiltinLayouts_IsZeroByteSet(t *testing.T) {
	assertZeroByteSet(t, "builtinLayouts", builtinLayouts)
}

func TestKnownFMKeys_IsZeroByteSet(t *testing.T) {
	assertZeroByteSet(t, "knownFMKeys", knownFMKeys)
}
