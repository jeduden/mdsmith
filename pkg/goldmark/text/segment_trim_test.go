package text_test

// Coverage for Segment slicing helpers that the parse flow only
// touches in specific input shapes: TrimLeftSpaceWidth across
// padding-consumption, space-consumption, tab-consumption, and
// negative-width-padding branches.

import (
	"testing"

	"github.com/yuin/goldmark/text"
)

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
