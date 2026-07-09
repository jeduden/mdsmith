package mdtext

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

func TestTOCItem_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(TOCItem{}))
}
