package index

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

func TestEdge_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(Edge{}))
}

func TestSymbol_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(Symbol{}))
}
