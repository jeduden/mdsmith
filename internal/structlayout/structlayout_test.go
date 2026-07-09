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
