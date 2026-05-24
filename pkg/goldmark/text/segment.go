package text

import (
	"bytes"

	"github.com/yuin/goldmark/util"
)

var space = []byte(" ")

// A Segment struct holds information about source positions.
type Segment struct {
	// Start is a start position of the segment.
	Start int

	// Stop is a stop position of the segment.
	// This value should be excluded.
	Stop int

	// Padding is a padding length of the segment.
	Padding int

	// ForceNewline is true if the segment should be ended with a newline.
	// Some elements(i.e. CodeBlock, FencedCodeBlock) does not trim trailing
	// newlines. Spec defines that EOF is treated as a newline, so we need to
	// add a newline to the end of the segment if it is not empty.
	//
	// i.e.:
	//
	//     ```go
	//     const test = "test"
	//
	// This code does not close the code block and ends with EOF. In this case,
	// we need to add a newline to the end of the last line like `const test = "test"\n`.
	ForceNewline bool
}

// NewSegment return a new Segment.
func NewSegment(start, stop int) Segment {
	return Segment{
		Start:   start,
		Stop:    stop,
		Padding: 0,
	}
}

// NewSegmentPadding returns a new Segment with the given padding.
func NewSegmentPadding(start, stop, n int) Segment {
	return Segment{
		Start:   start,
		Stop:    stop,
		Padding: n,
	}
}

// Value returns a value of the segment.
func (t *Segment) Value(buffer []byte) []byte {
	var result []byte
	if t.Padding == 0 {
		result = buffer[t.Start:t.Stop]
	} else {
		result = make([]byte, 0, t.Padding+t.Stop-t.Start+1)
		result = append(result, bytes.Repeat(space, t.Padding)...)
		result = append(result, buffer[t.Start:t.Stop]...)
	}
	if t.ForceNewline && len(result) > 0 && result[len(result)-1] != '\n' {
		result = append(result, '\n')
	}
	return result
}

// Len returns a length of the segment.
func (t *Segment) Len() int {
	return t.Stop - t.Start + t.Padding
}

// Between returns a segment between this segment and the given segment.
func (t *Segment) Between(other Segment) Segment {
	if t.Stop != other.Stop {
		panic("invalid state")
	}
	return NewSegmentPadding(
		t.Start,
		other.Start,
		t.Padding-other.Padding,
	)
}

// IsEmpty returns true if this segment is empty, otherwise false.
func (t *Segment) IsEmpty() bool {
	return t.Start >= t.Stop && t.Padding == 0
}

// TrimRightSpace returns a new segment by slicing off all trailing
// space characters.
func (t *Segment) TrimRightSpace(buffer []byte) Segment {
	v := buffer[t.Start:t.Stop]
	l := util.TrimRightSpaceLength(v)
	if l == len(v) {
		return NewSegment(t.Start, t.Start)
	}
	return NewSegmentPadding(t.Start, t.Stop-l, t.Padding)
}

// TrimLeftSpace returns a new segment by slicing off all leading
// space characters including padding.
func (t *Segment) TrimLeftSpace(buffer []byte) Segment {
	v := buffer[t.Start:t.Stop]
	l := util.TrimLeftSpaceLength(v)
	return NewSegment(t.Start+l, t.Stop)
}

// TrimLeftSpaceWidth returns a new segment by slicing off leading space
// characters until the given width.
func (t *Segment) TrimLeftSpaceWidth(width int, buffer []byte) Segment {
	padding := t.Padding
	for ; width > 0; width-- {
		if padding == 0 {
			break
		}
		padding--
	}
	if width == 0 {
		return NewSegmentPadding(t.Start, t.Stop, padding)
	}
	text := buffer[t.Start:t.Stop]
	start := t.Start
	for _, c := range text {
		if start >= t.Stop-1 || width <= 0 {
			break
		}
		if c == ' ' {
			width--
		} else if c == '\t' {
			width -= 4
		} else {
			break
		}
		start++
	}
	if width < 0 {
		padding = width * -1
	}
	return NewSegmentPadding(start, t.Stop, padding)
}

// WithStart returns a new Segment with same value except Start.
func (t *Segment) WithStart(v int) Segment {
	return NewSegmentPadding(v, t.Stop, t.Padding)
}

// WithStop returns a new Segment with same value except Stop.
func (t *Segment) WithStop(v int) Segment {
	return NewSegmentPadding(t.Start, v, t.Padding)
}

// ConcatPadding concats the padding to the given slice.
func (t *Segment) ConcatPadding(v []byte) []byte {
	if t.Padding > 0 {
		return append(v, bytes.Repeat(space, t.Padding)...)
	}
	return v
}

// SegmentsGrower lets an outside allocator redirect Segments
// backing-array growth. *Segments.Append calls Grow when its values
// slice has zero spare capacity; Grow returns a longer slice with
// the existing entries copied in and the new segment already
// appended. The arena package implements this so paragraph-line
// growth stays in arena memory instead of the runtime allocator —
// the fourth target in plan 197's allocator matrix. A nil grower
// keeps the upstream behaviour (plain `append`).
type SegmentsGrower interface {
	Grow(old []Segment, next Segment) []Segment
}

// Segments is a collection of the Segment.
//
// Fork divergence (plan 198): the embedded unexported `grow` field
// is fork-specific; it lets the arena redirect backing-array
// growth without changing the Append / AppendAll / Unshift call
// sites. Because the field is unexported, downstream code cannot
// reach it through composite literals — Go disallows unkeyed
// cross-package literals when any field is unexported, and there
// is no keyed access. Reflection or unsafe by struct-offset would
// see a different layout than upstream goldmark; mdsmith owns the
// only consumer in tree and does not need that.
type Segments struct {
	values []Segment
	grow   SegmentsGrower
}

// NewSegments return a new Segments.
func NewSegments() *Segments {
	return &Segments{
		values: nil,
	}
}

// newSegments returns a Segments allocated through the given
// SegmentsCreator (typically an arena), falling back to NewSegments
// when the creator is nil. Internal helper used by FindClosure so
// the choice lives in one spot.
func newSegments(c SegmentsCreator) *Segments {
	if c == nil {
		return NewSegments()
	}
	return c.Segments()
}

// SetBacking installs an arena-owned backing slice and grower on
// the receiver. After SetBacking, Append writes into `values` until
// its capacity is exhausted, then asks the grower for a longer
// slice. Used by `arena.Arena` when it constructs a Segments or
// when it equips a block node's embedded Lines() with arena-backed
// growth.
//
// SetBacking is safe to call on a fresh Segments only — calling it
// after Append would discard the existing entries. The arena calls
// SetBacking before handing the Segments back to the parser.
func (s *Segments) SetBacking(values []Segment, grow SegmentsGrower) {
	s.values = values
	s.grow = grow
}

// Append appends the given segment after the tail of the collection.
func (s *Segments) Append(t Segment) {
	if s.grow != nil && len(s.values) == cap(s.values) {
		s.values = s.grow.Grow(s.values, t)
		return
	}
	s.values = append(s.values, t)
}

// AppendAll appends all elements of given segments after the tail of the collection.
func (s *Segments) AppendAll(t []Segment) {
	if s.grow != nil {
		for _, seg := range t {
			s.Append(seg)
		}
		return
	}
	s.values = append(s.values, t...)
}

// Len returns the length of the collection.
func (s *Segments) Len() int {
	if s.values == nil {
		return 0
	}
	return len(s.values)
}

// At returns a segment at the given index.
func (s *Segments) At(i int) Segment {
	return s.values[i]
}

// Set sets the given Segment.
func (s *Segments) Set(i int, v Segment) {
	s.values[i] = v
}

// SetSliced replace the collection with a subsliced value.
func (s *Segments) SetSliced(lo, hi int) {
	s.values = s.values[lo:hi]
}

// Sliced returns a subslice of the collection.
func (s *Segments) Sliced(lo, hi int) []Segment {
	return s.values[lo:hi]
}

// Clear delete all element of the collection.
func (s *Segments) Clear() {
	s.values = nil
}

// Unshift inserts the given Segment at the head of the collection.
// Works on an empty receiver (where the upstream form
// `append(s.values[0:1], s.values[0:]...)` panicked). When the
// underlying array has spare capacity this implementation does
// not allocate: it grows the slice by one and uses copy() to
// shift the existing values right. copy() handles the alias case
// because Go's runtime uses memmove for overlapping ranges.
func (s *Segments) Unshift(v Segment) {
	if s.grow != nil && len(s.values) == cap(s.values) {
		s.values = s.grow.Grow(s.values, Segment{})
	} else {
		s.values = append(s.values, Segment{})
	}
	if len(s.values) > 1 {
		copy(s.values[1:], s.values[:len(s.values)-1])
	}
	s.values[0] = v
}

// Value returns a string value of the collection.
func (s *Segments) Value(buffer []byte) []byte {
	var result []byte
	for _, v := range s.values {
		result = append(result, v.Value(buffer)...)
	}
	return result
}
