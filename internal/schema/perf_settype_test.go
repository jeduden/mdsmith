package schema

// TestSetTypesAreStructNotBool documents the project's map[K]struct{}
// convention (docs/development/high-performance-go.md §Data structures)
// for the set-typed maps in this package. Each function call below is a
// compile-time assertion: the test will not compile if the named function
// returns map[K]bool instead of map[K]struct{}.
//
// Run: go test -run TestSetTypesAreStructNotBool ./internal/schema/

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestDocumentHeadingLinesReturnsStructSet(t *testing.T) {
	src := []byte("# Title\n\n## Section\n")
	f, err := lint.NewFile("t.md", src)
	if err != nil {
		t.Fatal(err)
	}
	// Compile-time check: the blank func call enforces the exact type without
	// triggering QF1011; if documentHeadingLines returned map[int]bool, this
	// would not compile.
	got := documentHeadingLines(f)
	func(_ map[int]struct{}) {}(got)
	if _, ok := got[1]; !ok {
		t.Fatal("expected line 1 (# Title) in heading-line set")
	}
	if _, ok := got[3]; !ok {
		t.Fatal("expected line 3 (## Section) in heading-line set")
	}
	if _, ok := got[2]; ok {
		t.Fatal("did not expect blank line 2 in heading-line set")
	}
}

func TestBuildKnownSetReturnsStructSet(t *testing.T) {
	got := buildKnownSet([]string{"A", "B"})
	// Compile-time check (see package doc above).
	func(_ map[string]struct{}) {}(got)
	if _, ok := got["A"]; !ok {
		t.Fatal("expected A in known set")
	}
	if _, ok := got["B"]; !ok {
		t.Fatal("expected B in known set")
	}
	if _, ok := got["C"]; ok {
		t.Fatal("did not expect C in known set")
	}
}

func TestDocumentSlugSetReturnsStructSet(t *testing.T) {
	src := []byte("# Hello World\n\n## Another Heading\n")
	f, err := lint.NewFile("t.md", src)
	if err != nil {
		t.Fatal(err)
	}
	got := documentSlugSet(f)
	// Compile-time check (see package doc above).
	func(_ map[string]struct{}) {}(got)
	if _, ok := got["hello-world"]; !ok {
		t.Fatalf("expected slug hello-world in set, got %v", got)
	}
	if _, ok := got["another-heading"]; !ok {
		t.Fatalf("expected slug another-heading in set, got %v", got)
	}
}
