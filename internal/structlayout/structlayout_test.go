package structlayout

import (
	"fmt"
	"reflect"
	"strings"
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

type multiBadLayout struct {
	A string
	X int
	B string
	Y int
	C string
}

type nestedGood struct {
	Inner goodLayout
	Count int
}

type allScalar struct {
	A int
	B bool
}

type nestedAllScalar struct {
	Inner allScalar
	Name  string
}

// fakeReporter records Errorf calls instead of failing the real test,
// so AssertPointerFieldsFirst's reporting branch is directly testable.
type fakeReporter struct {
	helperCalled bool
	messages     []string
}

func (f *fakeReporter) Helper() { f.helperCalled = true }

func (f *fakeReporter) Errorf(format string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

func TestAssertPointerFieldsFirst_NoViolationOnGoodLayout(t *testing.T) {
	fr := &fakeReporter{}
	AssertPointerFieldsFirst(fr, reflect.TypeOf(goodLayout{}))
	if !fr.helperCalled {
		t.Error("expected Helper() to be called")
	}
	if len(fr.messages) != 0 {
		t.Fatalf("expected no violations, got %v", fr.messages)
	}
}

func TestAssertPointerFieldsFirst_NoViolationOnNestedGood(t *testing.T) {
	fr := &fakeReporter{}
	AssertPointerFieldsFirst(fr, reflect.TypeOf(nestedGood{}))
	if len(fr.messages) != 0 {
		t.Fatalf("expected no violations, got %v", fr.messages)
	}
}

func TestAssertPointerFieldsFirst_ReportsSingleViolation(t *testing.T) {
	fr := &fakeReporter{}
	AssertPointerFieldsFirst(fr, reflect.TypeOf(badLayout{}))
	if len(fr.messages) != 1 {
		t.Fatalf("expected exactly 1 violation, got %v", fr.messages)
	}
	if !strings.Contains(fr.messages[0], "badLayout.Name") {
		t.Fatalf("expected message naming badLayout.Name, got %q", fr.messages[0])
	}
}

func TestAssertPointerFieldsFirst_ReportsEveryViolation(t *testing.T) {
	fr := &fakeReporter{}
	AssertPointerFieldsFirst(fr, reflect.TypeOf(multiBadLayout{}))
	if len(fr.messages) != 2 {
		t.Fatalf("expected 2 violations (B after X, C after X), got %v", fr.messages)
	}
}

// TestIsPointerish_StructWithNoPointerFields pins the reflect.Struct
// branch's fallthrough: a struct field whose own fields are all
// scalar is itself scalar, not pointerish. So in nestedAllScalar,
// Inner counts as a scalar field, and Name (a string, declared after
// it) is correctly flagged as an out-of-order pointer field.
func TestIsPointerish_StructWithNoPointerFields(t *testing.T) {
	if isPointerish(reflect.TypeOf(allScalar{})) {
		t.Error("allScalar: expected not pointerish (every field is scalar)")
	}
	fr := &fakeReporter{}
	AssertPointerFieldsFirst(fr, reflect.TypeOf(nestedAllScalar{}))
	if len(fr.messages) != 1 {
		t.Fatalf("expected 1 violation (Name declared after the scalar Inner field), got %v", fr.messages)
	}
	if !strings.Contains(fr.messages[0], "nestedAllScalar.Name") {
		t.Fatalf("expected message naming nestedAllScalar.Name, got %q", fr.messages[0])
	}
}

// TestIsPointerish_Array pins the reflect.Array branch: a non-empty
// array of a pointer-containing element type is pointerish, but a
// zero-length array is not, regardless of its element type — there
// is no element to scan, so it costs nothing whichever type it
// declares.
func TestIsPointerish_Array(t *testing.T) {
	if !isPointerish(reflect.TypeOf([2]string{})) {
		t.Error("[2]string: expected pointerish (non-empty array of a pointer-containing element)")
	}
	if isPointerish(reflect.TypeOf([0]string{})) {
		t.Error("[0]string: expected not pointerish (zero-length array scans nothing)")
	}
	if isPointerish(reflect.TypeOf([2]int{})) {
		t.Error("[2]int: expected not pointerish (scalar element type)")
	}
}
