package structlayout

import (
	"reflect"
	"testing"
)

type goodLayout struct {
	Name  string
	Level int
}

type badLayout struct {
	Level int
	Name  string
}

type nestedGood struct {
	Inner goodLayout
	Count int
}

func TestFirstOutOfOrderField_Good(t *testing.T) {
	if field, bad := firstOutOfOrderField(reflect.TypeOf(goodLayout{})); bad {
		t.Fatalf("expected no violation, got field %q", field)
	}
	if field, bad := firstOutOfOrderField(reflect.TypeOf(nestedGood{})); bad {
		t.Fatalf("expected no violation, got field %q", field)
	}
}

func TestFirstOutOfOrderField_Bad(t *testing.T) {
	field, bad := firstOutOfOrderField(reflect.TypeOf(badLayout{}))
	if !bad {
		t.Fatal("expected a violation on badLayout (Name declared after Level)")
	}
	if field != "Name" {
		t.Fatalf("expected offending field %q, got %q", "Name", field)
	}
}

func TestAssertPointerFieldsFirst_PassesOnGoodLayout(t *testing.T) {
	AssertPointerFieldsFirst(t, reflect.TypeOf(goodLayout{}))
}
