package refactor

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jeduden/mdsmith/internal/mdtext"
)

// ApplyEdits splices every edit into src and returns the rewritten
// bytes. It is a pure in-memory transform: the host reads src and
// writes the result. When any edit breaks the rules below it returns
// nil and an error — never a partially edited result.
//
// Positions use the LSP model the planners emit: a zero-based Line and
// a Character counted in UTF-16 code units, the range half-open
// [Start, End).
//
//   - An edit is single-line (heading text, label, or fragment); a
//     range spanning lines is an error.
//   - Line must name a line of src, and 0 <= Start <= End <= the
//     line's UTF-16 length. A Character outside the line is an error,
//     not clamped to the nearer end: every planner derives Characters
//     from the line it edits, so one outside it means the plan is
//     wrong or stale. A CRLF line's `\r` is not addressable; it is
//     preserved so CRLF files round-trip.
//   - No byte of the original line may be claimed by two edits. Two
//     edits whose ranges share a byte are an error naming the line and
//     both ranges — a partial overlap, a containment, a zero-width
//     insert strictly inside another edit's range, and two identical
//     replacements alike. Identical replacements are rejected rather
//     than deduplicated: they mean a planner visited one token twice,
//     and LSP forbids the same plan as overlapping.
//   - Edits may touch. An insert at a replacement's start or end, and
//     adjacent replacements, all apply.
//   - Zero-width inserts at one offset all apply, in input order, and
//     land before a replacement starting at that offset. Two identical
//     inserts of "x" yield "xx": each is a distinct insert, as in LSP.
//
// The overlap and same-offset rules are LSP's TextEdit rules, so a Plan
// lands the same here as when an editor applies it; only the position
// check is stricter than LSP's clamp. Lines are checked in document
// order, so a plan with several bad lines always names the first.
func ApplyEdits(src []byte, edits []Edit) ([]byte, error) {
	segs := splitKeepCR(src)
	byLine := map[int][]Edit{}
	for _, e := range edits {
		if e.Range.Start.Line != e.Range.End.Line {
			return nil, errors.New("multi-line edit is not supported")
		}
		byLine[e.Range.Start.Line] = append(byLine[e.Range.Start.Line], e)
	}
	// Visit lines in document order, not map order, so a plan with
	// several bad lines always reports the same (first) one.
	for _, line := range slices.Sorted(maps.Keys(byLine)) {
		if line < 0 || line >= len(segs) {
			return nil, fmt.Errorf("edit line %d out of range", line+1)
		}
		out, err := spliceLine(segs[line], byLine[line], line)
		if err != nil {
			return nil, err
		}
		segs[line] = out
	}
	return joinLF(segs), nil
}

// spliceLine applies es — every edit on zero-based line — to seg, that
// line's bytes including any trailing `\r`, and returns the rewritten
// line. It sorts es by range, checks each edit against the line and
// against the edit before it, then builds the result in one
// left-to-right pass: the bytes before each edit, its NewText, and the
// bytes after the last. Because accepted edits never overlap, each
// edit's end is at or past every earlier one's, so comparing with the
// previous edit alone finds any overlap.
func spliceLine(seg []byte, es []Edit, line int) ([]byte, error) {
	row := seg
	cr := len(seg) > 0 && seg[len(seg)-1] == '\r'
	if cr {
		row = seg[:len(seg)-1]
	}
	rowLen := mdtext.UTF16FromByteOffset(row, len(row))
	sortEditsByRange(es)
	out := make([]byte, 0, len(seg))
	copied := 0 // row bytes before copied are already in out
	for i, e := range es {
		if err := checkEditRange(e, rowLen, line); err != nil {
			return nil, err
		}
		if i > 0 {
			if prev := es[i-1].Range; e.Range.Start.Character < prev.End.Character {
				return nil, fmt.Errorf("edits [%d,%d) and [%d,%d) on line %d overlap",
					prev.Start.Character, prev.End.Character,
					e.Range.Start.Character, e.Range.End.Character, line+1)
			}
		}
		out = append(out, row[copied:mdtext.UTF16ToByteOffset(row, e.Range.Start.Character)]...)
		out = append(out, e.NewText...)
		copied = mdtext.UTF16ToByteOffset(row, e.Range.End.Character)
	}
	out = append(out, row[copied:]...)
	if cr {
		out = append(out, '\r')
	}
	return out, nil
}

// checkEditRange reports an error when e's Characters do not address a
// valid range of a row rowLen UTF-16 units long: 0 <= Start <= End <=
// rowLen. line is zero-based; the error names it one-based.
func checkEditRange(e Edit, rowLen, line int) error {
	start, end := e.Range.Start.Character, e.Range.End.Character
	if start > end {
		return fmt.Errorf("edit [%d,%d) on line %d ends before it starts", start, end, line+1)
	}
	if start < 0 || end > rowLen {
		return fmt.Errorf("edit [%d,%d) on line %d is outside the line (length %d)", start, end, line+1, rowLen)
	}
	return nil
}

// sortEditsByRange orders es in place by Start.Character, then by
// End.Character, so a zero-width insert sorts before a replacement
// that starts at the same offset. The sort is stable: edits with equal
// ranges — inserts at one offset — keep their input order, which is
// the order their text lands in. slices.SortStableFunc compares the
// concrete Edit values directly, unlike sort.SliceStable, which drives
// reflect.Swapper under the hood — see
// docs/development/high-performance-go.md's "reflect in hot paths"
// anti-pattern.
func sortEditsByRange(es []Edit) {
	slices.SortStableFunc(es, func(a, b Edit) int {
		if c := cmp.Compare(a.Range.Start.Character, b.Range.Start.Character); c != 0 {
			return c
		}
		return cmp.Compare(a.Range.End.Character, b.Range.End.Character)
	})
}

// splitKeepCR splits src on `\n`, keeping any trailing `\r` on each
// segment so CRLF endings survive a round-trip.
func splitKeepCR(src []byte) [][]byte {
	var segs [][]byte
	start := 0
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			segs = append(segs, src[start:i])
			start = i + 1
		}
	}
	segs = append(segs, src[start:])
	return segs
}

// joinLF rejoins segments with `\n`, the inverse of splitKeepCR.
func joinLF(segs [][]byte) []byte {
	var out []byte
	for i, s := range segs {
		if i > 0 {
			out = append(out, '\n')
		}
		out = append(out, s...)
	}
	return out
}
