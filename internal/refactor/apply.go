package refactor

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"

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
	for _, line := range slices.Sorted(maps.Keys(byLine)) {
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

// resolvedEdit is an Edit with its Range mapped from UTF-16 characters
// to byte offsets in one line, dropping the Line field (the caller
// already grouped by line). Sorting, deduping, and conflict detection
// all key on (s, en) rather than the raw Range so that two edits whose
// Character values differ but clamp to the same byte offset (both
// past end of line, say) are compared on the same footing as two
// edits that share a Character value outright — see
// applyLineEdits's reversed-range comment for why UTF16ToByteOffset's
// clamp makes that distinction matter.
type resolvedEdit struct {
	s, en int
	text  string
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
	resolved := make([]resolvedEdit, len(es))
	for i, e := range es {
		// Reject a negative Character on the raw values, before
		// UTF16ToByteOffset's target<=0 guard silently treats it the same
		// as 0. Unlike a Character past end of line (a deliberately
		// tolerated LSP-style position, clamped to len(row) by design —
		// see the EOL-clamp test in apply_test.go), a negative Character
		// has no valid interpretation: it names a position before the
		// line even starts, so there is no permissive reading to fall
		// back on.
		if e.Range.Start.Character < 0 || e.Range.End.Character < 0 {
			return nil, fmt.Errorf("edit range [%d,%d) has a negative Character on line %d",
				e.Range.Start.Character, e.Range.End.Character, line+1)
		}
		// Reject a reversed Start/End pair on the raw character values,
		// before UTF16ToByteOffset clamps them. UTF16ToByteOffset is
		// monotonic (never returns a smaller byte offset for a larger
		// character offset) and clamps to len(row), so checking the
		// post-clamp s > en instead would miss a reversed pair that both
		// land past end of line — clamping collapses both to len(row),
		// making them look like a valid zero-width edit there.
		if e.Range.Start.Character > e.Range.End.Character {
			return nil, fmt.Errorf("edit range [%d,%d) is reversed on line %d",
				e.Range.Start.Character, e.Range.End.Character, line+1)
		}
		resolved[i] = resolvedEdit{
			s:    mdtext.UTF16ToByteOffset(row, e.Range.Start.Character),
			en:   mdtext.UTF16ToByteOffset(row, e.Range.End.Character),
			text: e.NewText,
		}
	}
	sortResolvedEditsAsc(resolved)
	resolved = dedupeIdenticalResolvedEdits(resolved)
	buf := make([]byte, 0, len(row))
	pos := 0
	havePrev := false
	var prevS, prevEn int
	for _, r := range resolved {
		// dedupeIdenticalResolvedEdits already dropped every duplicate
		// with matching text, so a (s, en) pair this loop sees twice
		// means two edits disagree on what to put at that exact byte
		// range — an ambiguous conflict a numeric pos check alone would
		// miss for a zero-width range (it never advances pos past its
		// own start, so a second zero-width edit at the same point
		// looks merely adjacent rather than overlapping).
		if havePrev && r.s == prevS && r.en == prevEn {
			return nil, fmt.Errorf(
				"conflicting edits at byte %d on line %d: same range, different text", r.s, line+1)
		}
		if r.s < pos {
			return nil, fmt.Errorf("overlapping edits at byte %d on line %d", r.s, line+1)
		}
		buf = append(buf, row[pos:r.s]...)
		buf = append(buf, r.text...)
		pos = r.en
		prevS, prevEn, havePrev = r.s, r.en, true
	}
	buf = append(buf, row[pos:]...)
	if cr {
		buf = append(buf, '\r')
	}
	return buf, nil
}

// sortResolvedEditsAsc orders res by ascending (s, en) in place, so
// applyLineEdits can build the rewritten line in a single left-to-right
// pass and detect a conflict or overlap by comparing each edit's start
// against the previous edit's end. The en tie-break matters when two
// edits share s: it puts the narrower edit first, so a zero-width
// insert always sorts before a wider edit that starts at the same
// byte, regardless of which order the caller reported them in —
// without it, that pairing could sort either way and the insert could
// come out looking like it overlaps the wider edit.
// slices.SortStableFunc compares the concrete resolvedEdit values
// directly, unlike sort.SliceStable, which drives reflect.Swapper
// under the hood — see docs/development/high-performance-go.md's
// "reflect in hot paths" anti-pattern. Stability preserves the
// original order among edits reported at the same (s, en) (resolved by
// dedupeIdenticalResolvedEdits when they also share text, or by the
// conflict check otherwise).
func sortResolvedEditsAsc(res []resolvedEdit) {
	slices.SortStableFunc(res, func(a, b resolvedEdit) int {
		if c := cmp.Compare(a.s, b.s); c != 0 {
			return c
		}
		return cmp.Compare(a.en, b.en)
	})
}

// dedupeIdenticalResolvedEdits drops adjacent entries that share the
// same (s, en, text), keeping the first. res must already be sorted by
// sortResolvedEditsAsc, which places identical-range entries next to
// each other. A caller-side index or graph walk occasionally reports
// the same rewrite twice (e.g. an anchor and a path edit resolving to
// the same edit); treating an exact duplicate as a no-op instead of an
// overlap error keeps ApplyEdits tolerant of that without masking a
// genuine conflict — two entries left at the same (s, en) with
// different text still fail applyLineEdits's conflicting-range check.
// Only adjacent duplicates merge, but that is enough: three entries at
// one range with text A, B, A leave the middle B unmerged, and the
// conflicting-range check catches the mismatch against B before either
// A ever needs comparing to the other.
//
// Deduping compares the resolved (s, en), not the caller's raw
// Character values, deliberately: two edits that write the identical
// text to the identical byte span produce byte-identical output
// whether or not their raw Characters matched before clamping, so
// merging them changes nothing about what reaches disk. The only
// caller-visible effect is the reported edit count, which can then
// undercount an adversarial or malformed edit list (e.g. two
// out-of-range Characters that both clamp to end of line) — a
// cosmetic gap in a rarely-hit edge case, not a correctness one; no
// real Heading, LinkRef, or Move plan produces Characters outside the
// line it was parsed from, so this never triggers on legitimate input.
func dedupeIdenticalResolvedEdits(res []resolvedEdit) []resolvedEdit {
	if len(res) < 2 {
		return res
	}
	out := res[:1]
	for _, r := range res[1:] {
		last := out[len(out)-1]
		if r.s == last.s && r.en == last.en && r.text == last.text {
			continue
		}
		out = append(out, r)
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
