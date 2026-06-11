package schema

// TestSetTypesAreStructNotBool documents the project's map[K]struct{}
// convention (docs/development/high-performance-go.md §Data structures)
// for the set-typed maps in this package. Each variable assignment below
// is a compile-time assertion: the test will not compile if the named
// function returns map[K]bool instead of map[K]struct{}.
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
	// Compile-time check: documentHeadingLines must return map[int]struct{}.
	var _ map[int]struct{} = documentHeadingLines(f)
}

func TestBuildKnownSetReturnsStructSet(t *testing.T) {
	// Compile-time check: buildKnownSet must return map[string]struct{}.
	var _ map[string]struct{} = buildKnownSet([]string{"A", "B"})
	got := buildKnownSet([]string{"A", "B"})
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
	// Compile-time check: documentSlugSet must return map[string]struct{}.
	var _ map[string]struct{} = documentSlugSet(f)
	got := documentSlugSet(f)
	if _, ok := got["hello-world"]; !ok {
		t.Fatalf("expected slug hello-world in set, got %v", got)
	}
	if _, ok := got["another-heading"]; !ok {
		t.Fatalf("expected slug another-heading in set, got %v", got)
	}
}
