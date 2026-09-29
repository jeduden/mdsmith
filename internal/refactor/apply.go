package refactor

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/jeduden/mdsmith/internal/mdtext"
)

// ApplyEdits splices every edit into src and returns the rewritten
// bytes. Each edit must be single-line (heading text, label, or
// fragment) — Plan never produces multi-line edits, since a rename
// only ever rewrites text within one line. Edits on the same line
// must not overlap (including two edits at an identical range); an
// overlap returns an error rather than silently corrupting the line.
// A trailing `\r` is preserved so CRLF files round-trip.
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
	for line, es := range byLine {
		if line < 0 || line >= len(segs) {
			return nil, fmt.Errorf("edit line %d out of range", line+1)
		}
		seg := segs[line]
		cr := len(seg) > 0 && seg[len(seg)-1] == '\r'
		row := seg
		if cr {
			row = seg[:len(seg)-1]
		}
		sortEditsByCharacterAsc(es)
		buf := make([]byte, 0, len(row))
		pos := 0
		for _, e := range es {
			// UTF16ToByteOffset always returns a value in [0, len(row)],
			// so the only reachable invalid case is s > en (a reversed
			// Start/End pair) or an overlap with the previous edit.
			s := mdtext.UTF16ToByteOffset(row, e.Range.Start.Character)
			en := mdtext.UTF16ToByteOffset(row, e.Range.End.Character)
			if s > en {
				return nil, fmt.Errorf("edit offset [%d,%d) out of range on line %d", s, en, line+1)
			}
			if s < pos {
				return nil, fmt.Errorf("overlapping edits at byte %d on line %d", s, line+1)
			}
			buf = append(buf, row[pos:s]...)
			buf = append(buf, e.NewText...)
			pos = en
		}
		buf = append(buf, row[pos:]...)
		if cr {
			buf = append(buf, '\r')
		}
		segs[line] = buf
	}
	return joinLF(segs), nil
}

// sortEditsByCharacterAsc orders es by ascending Start.Character in
// place, so ApplyEdits can build the rewritten line in a single
// left-to-right pass and detect an overlap by comparing each edit's
// start against the previous edit's end. slices.SortStableFunc
// compares the concrete Edit values directly, unlike sort.SliceStable,
// which drives reflect.Swapper under the hood — see
// docs/development/high-performance-go.md's "reflect in hot paths"
// anti-pattern. Stability preserves the original order among edits
// reported at the same offset (which then fails the overlap check).
func sortEditsByCharacterAsc(es []Edit) {
	slices.SortStableFunc(es, func(a, b Edit) int {
		return cmp.Compare(a.Range.Start.Character, b.Range.Start.Character)
	})
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
