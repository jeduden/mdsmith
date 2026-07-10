package schema

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

func TestScopeMatch_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(ScopeMatch{}))
}

func TestDocHeading_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(DocHeading{}))
}
