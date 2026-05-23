package text_test

import (
	"testing"

	"github.com/yuin/goldmark/text"
)

func TestSegments_Unshift_Empty(t *testing.T) {
	// Empty case must not panic.
	var s text.Segments
	s.Unshift(text.NewSegment(0, 5))
	if s.Len() != 1 {
		t.Fatalf("Len after Unshift on empty = %d, want 1", s.Len())
	}
	if got := s.At(0); got.Start != 0 || got.Stop != 5 {
		t.Errorf("At(0) = %+v, want [0,5)", got)
	}
}

func TestSegments_Unshift_PrependsToHead(t *testing.T) {
	var s text.Segments
	s.Append(text.NewSegment(10, 20))
	s.Append(text.NewSegment(20, 30))
	s.Unshift(text.NewSegment(0, 10))
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3", s.Len())
	}
	got := s.At(0)
	if got.Start != 0 || got.Stop != 10 {
		t.Errorf("head segment = %+v, want [0,10)", got)
	}
	tail := s.At(2)
	if tail.Start != 20 || tail.Stop != 30 {
		t.Errorf("tail segment = %+v, want [20,30)", tail)
	}
}
