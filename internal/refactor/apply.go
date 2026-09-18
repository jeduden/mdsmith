package refactor

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/jeduden/mdsmith/internal/mdtext"
)

// ApplyEdits splices every edit into src and returns the rewritten
// bytes. Each edit must be single-line (heading text, label, or
// fragment) — Plan never produces multi-line edits, since a rename
// only ever rewrites text within one line. Edits on the same line are
// applied right-to-left so a left edit's byte offsets — computed
// against the original row — stay valid while the bytes to its right
// are rewritten. A trailing `\r` is preserved so CRLF files round-trip.
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
		sortEditsByCharacterDesc(es)
		buf := append([]byte(nil), row...)
		for _, e := range es {
			s := mdtext.UTF16ToByteOffset(row, e.Range.Start.Character)
			en := mdtext.UTF16ToByteOffset(row, e.Range.End.Character)
			if s < 0 || en < 0 || s > len(buf) || en > len(buf) || s > en {
				return nil, fmt.Errorf("edit offset [%d,%d) out of range on line %d", s, en, line+1)
			}
			next := make([]byte, 0, len(buf)-(en-s)+len(e.NewText))
			next = append(next, buf[:s]...)
			next = append(next, e.NewText...)
			next = append(next, buf[en:]...)
			buf = next
		}
		if cr {
			buf = append(buf, '\r')
		}
		segs[line] = buf
	}
	return joinLF(segs), nil
}

// sortEditsByCharacterDesc orders es by descending Start.Character in
// place (rightmost edit first), so ApplyEdits can splice each edit
// into the line without its offset shifting from an earlier splice.
// slices.SortStableFunc compares the concrete Edit values directly,
// unlike sort.SliceStable, which drives reflect.Swapper under the
// hood — see docs/development/high-performance-go.md's "reflect in
// hot paths" anti-pattern. Stability preserves the original order
// among edits reported at the same offset.
func sortEditsByCharacterDesc(es []Edit) {
	slices.SortStableFunc(es, func(a, b Edit) int {
		return cmp.Compare(b.Range.Start.Character, a.Range.Start.Character)
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
