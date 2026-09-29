package refactor

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/jeduden/mdsmith/internal/mdtext"
)

// ApplyEdits splices every edit into src and returns the rewritten
// bytes. Each edit must be single-line (heading text, label, or
// fragment) — Plan never produces multi-line edits, since a rename
// only ever rewrites text within one line. Two edits on the same line
// at an identical range (same Start, End, and NewText — the same
// change reported twice, e.g. by an index that doesn't dedup) collapse
// into one. Two edits at the same range with different NewText — an
// ambiguous conflict, including two zero-width inserts at the same
// point — return an error, as does any other overlap, rather than
// silently corrupting the line. A trailing `\r` is preserved so CRLF
// files round-trip.
//
// ApplyEdits is a pure, in-memory transform; it never touches the
// filesystem, matching this package's "the engine never touches the
// filesystem" contract (see the package doc comment). A host applies
// the result to disk (or a buffer) itself.
func ApplyEdits(src []byte, edits []Edit) ([]byte, error) {
	segs := splitKeepCR(src)
	byLine := map[int][]Edit{}
	for _, e := range edits {
		if e.Range.Start.Line != e.Range.End.Line {
			return nil, errors.New("multi-line edit is not supported")
		}
		byLine[e.Range.Start.Line] = append(byLine[e.Range.Start.Line], e)
	}
	// Iterate in line order so the returned error is deterministic when
	// more than one line is bad.
	lines := make([]int, 0, len(byLine))
	for line := range byLine {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	for _, line := range lines {
		if line < 0 || line >= len(segs) {
			return nil, fmt.Errorf("edit line %d out of range", line+1)
		}
		buf, err := applyLineEdits(segs[line], byLine[line], line)
		if err != nil {
			return nil, err
		}
		segs[line] = buf
	}
	return joinLF(segs), nil
}

// applyLineEdits splices es into seg (one line, with its trailing `\r`
// if any) and returns the rewritten line. line is the 0-based source
// line, used only to name it in an error.
func applyLineEdits(seg []byte, es []Edit, line int) ([]byte, error) {
	cr := len(seg) > 0 && seg[len(seg)-1] == '\r'
	row := seg
	if cr {
		row = seg[:len(seg)-1]
	}
	sortEditsByCharacterAsc(es)
	es = dedupeIdenticalEdits(es)
	buf := make([]byte, 0, len(row))
	pos := 0
	havePrev := false
	var prevRange Range
	for _, e := range es {
		// UTF16ToByteOffset always returns a value in [0, len(row)], so
		// the only reachable invalid case is s > en (a reversed
		// Start/End pair) or an overlap with the previous edit.
		s := mdtext.UTF16ToByteOffset(row, e.Range.Start.Character)
		en := mdtext.UTF16ToByteOffset(row, e.Range.End.Character)
		if s > en {
			return nil, fmt.Errorf("edit offset [%d,%d) out of range on line %d", s, en, line+1)
		}
		// dedupeIdenticalEdits already dropped every duplicate with
		// matching NewText, so a Range this loop sees twice means two
		// edits disagree on what to put at that exact range — an
		// ambiguous conflict a numeric pos check alone would miss for a
		// zero-width range (it never advances pos past its own start, so
		// a second zero-width edit at the same point looks merely
		// adjacent rather than overlapping).
		if havePrev && e.Range == prevRange {
			return nil, fmt.Errorf(
				"conflicting edits at byte %d on line %d: same range, different text", s, line+1)
		}
		if s < pos {
			return nil, fmt.Errorf("overlapping edits at byte %d on line %d", s, line+1)
		}
		buf = append(buf, row[pos:s]...)
		buf = append(buf, e.NewText...)
		pos = en
		prevRange, havePrev = e.Range, true
	}
	buf = append(buf, row[pos:]...)
	if cr {
		buf = append(buf, '\r')
	}
	return buf, nil
}

// sortEditsByCharacterAsc orders es by ascending (Start.Character,
// End.Character) in place, so ApplyEdits can build the rewritten line
// in a single left-to-right pass and detect an overlap by comparing
// each edit's start against the previous edit's end. The End tie-break
// matters when two edits share a Start: it puts the narrower edit
// first, so a zero-width insert always sorts before a wider edit that
// starts at the same point, regardless of which order the caller
// reported them in — without it, that pairing could sort either way
// and the insert could come out looking like it overlaps the wider
// edit. slices.SortStableFunc compares the concrete Edit values
// directly, unlike sort.SliceStable, which drives reflect.Swapper
// under the hood — see docs/development/high-performance-go.md's
// "reflect in hot paths" anti-pattern. Stability preserves the
// original order among edits reported at the same (Start, End)
// (resolved by dedupeIdenticalEdits when they also share NewText, or
// by the overlap check otherwise).
func sortEditsByCharacterAsc(es []Edit) {
	slices.SortStableFunc(es, func(a, b Edit) int {
		if c := cmp.Compare(a.Range.Start.Character, b.Range.Start.Character); c != 0 {
			return c
		}
		return cmp.Compare(a.Range.End.Character, b.Range.End.Character)
	})
}

// dedupeIdenticalEdits drops adjacent edits that share the same Start,
// End, and NewText, keeping the first. es must already be sorted by
// sortEditsByCharacterAsc, which places identical-range edits next to
// each other. A caller-side index or graph walk occasionally reports
// the same rewrite twice (e.g. an anchor and a path edit resolving to
// the same edit); treating an exact duplicate as a no-op instead of an
// overlap error keeps ApplyEdits tolerant of that without masking a
// genuine conflict — two edits left at the same range with different
// NewText still fail the caller's conflicting-range check.
func dedupeIdenticalEdits(es []Edit) []Edit {
	if len(es) < 2 {
		return es
	}
	out := es[:1]
	for _, e := range es[1:] {
		last := out[len(out)-1]
		if e.Range == last.Range && e.NewText == last.NewText {
			continue
		}
		out = append(out, e)
	}
	return out
}

// splitKeepCR splits src on `\n`, keeping any trailing `\r` on each
// segment so CRLF endings survive a round-trip.
func splitKeepCR(src []byte) [][]byte {
	segs := make([][]byte, 0, bytes.Count(src, []byte{'\n'})+1)
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
	size := len(segs) - 1
	for _, s := range segs {
		size += len(s)
	}
	out := make([]byte, 0, size)
	for i, s := range segs {
		if i > 0 {
			out = append(out, '\n')
		}
		out = append(out, s...)
	}
	return out
}
