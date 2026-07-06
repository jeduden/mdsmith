package fieldorder

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// nestedPointerish is a struct whose Kind is reflect.Struct but which
// carries a pointer word (Data) via a nested field — the shape
// IsPointerish must recognise even though the field itself is not
// directly a pointer/interface/slice/map/chan/func/string/
// unsafe.Pointer kind.
type nestedPointerish struct {
	Data []byte
}

func TestIsPointerish_DetectsNestedStructField(t *testing.T) {
	typ := reflect.TypeOf(nestedPointerish{})
	assert.True(t, IsPointerish(typ),
		"a struct containing a pointer-bearing field (here, a []byte) must itself be pointer-ish")
}

func TestIsPointerish_DetectsArrayOfPointerish(t *testing.T) {
	typ := reflect.TypeOf([2]nestedPointerish{})
	assert.True(t, IsPointerish(typ),
		"an array of pointer-bearing elements must itself be pointer-ish")
}

func TestIsPointerish_ScalarStructIsNotPointerish(t *testing.T) {
	type allScalar struct {
		A int
		B bool
		C byte
	}
	assert.False(t, IsPointerish(reflect.TypeOf(allScalar{})))
}

func TestIsPointerish_DirectKinds(t *testing.T) {
	assert.True(t, IsPointerish(reflect.TypeOf((*int)(nil))))
	assert.True(t, IsPointerish(reflect.TypeOf("")))
	assert.True(t, IsPointerish(reflect.TypeOf([]int(nil))))
	assert.False(t, IsPointerish(reflect.TypeOf(0)))
	assert.False(t, IsPointerish(reflect.TypeOf(byte(0))))
}
