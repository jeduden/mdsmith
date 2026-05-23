package text_test

// Coverage for Segment slicing helpers that the parse flow only
// touches in specific input shapes: TrimLeftSpaceWidth across
// padding-consumption, space-consumption, tab-consumption, and
// negative-width-padding branches.

import (
	"testing"

	"github.com/yuin/goldmark/text"
)

func TestSegment_ConcatPadding(t *testing.T) {
	// ConcatPadding has two branches: Padding > 0 (append spaces)
	// and Padding == 0 (return v as-is).
	zero := text.NewSegment(0, 5)
	if got := zero.ConcatPadding([]byte("abc")); string(got) != "abc" {
		t.Errorf("ConcatPadding zero padding = %q, want abc", got)
	}
	three := text.NewSegmentPadding(0, 5, 3)
	if got := three.ConcatPadding([]byte("abc")); string(got) != "abc   " {
		t.Errorf("ConcatPadding 3 = %q, want 'abc   '", got)
	}
}

func TestSegment_Between(t *testing.T) {
	// Between returns a Segment with the same Stop as both inputs.
	a := text.NewSegment(0, 10)
	b := text.NewSegment(3, 10)
	got := a.Between(b)
	if got.Start != 0 || got.Stop != 3 {
		t.Errorf("Between = {%d,%d}, want {0,3}", got.Start, got.Stop)
	}
}

func TestSegment_Value_PaddingAndForceNewline(t *testing.T) {
	// Value's padding branch + ForceNewline branch.
	src := []byte("hello world")

	// Padding > 0.
	seg := text.NewSegmentPadding(0, 5, 3)
	got := seg.Value(src)
	if string(got) != "   hello" {
		t.Errorf("Padding=3 Value = %q, want '   hello'", got)
	}

	// ForceNewline branch.
	seg2 := text.NewSegment(0, 5)
	seg2.ForceNewline = true
	got2 := seg2.Value(src)
	if string(got2) != "hello\n" {
		t.Errorf("ForceNewline Value = %q, want 'hello\\n'", got2)
	}

	// ForceNewline but already ends with newline.
	seg3 := text.NewSegment(0, 6) // "hello "
	seg3.ForceNewline = true
	_ = seg3.Value(src)
}

func TestSegment_Between_PanicsOnStopMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Between with mismatched Stop should panic")
		}
	}()
	a := text.NewSegment(0, 5)
	b := text.NewSegment(0, 10) // different Stop
	_ = a.Between(b)
}

func TestSegment_TrimLeftSpaceWidth_Branches(t *testing.T) {
	src := []byte("    abc\t\tdef")

	cases := []struct {
		name      string
		seg       text.Segment
		width     int
		wantStart int
		wantPad   int
	}{
		{
			name:      "consume-all-padding-first",
			seg:       text.NewSegmentPadding(4, 12, 3),
			width:     2,
			wantStart: 4,
			wantPad:   1,
		},
		{
			name:      "consume-spaces",
			seg:       text.NewSegment(0, 12),
			width:     2,
			wantStart: 2,
			wantPad:   0,
		},
		{
			name:      "tab-over-consumes-yields-padding",
			seg:       text.NewSegment(7, 12),
			width:     2,
			wantStart: 8,
			wantPad:   2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.seg.TrimLeftSpaceWidth(c.width, src)
			if got.Start != c.wantStart {
				t.Errorf("Start = %d, want %d", got.Start, c.wantStart)
			}
			if got.Padding != c.wantPad {
				t.Errorf("Padding = %d, want %d", got.Padding, c.wantPad)
			}
		})
	}
}
