package foreignregion

import (
	"bytes"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
)

// Restore overwrites, in fixed, the bytes of every foreign region with
// the corresponding bytes from original, so no fixer's edit inside a
// marker pair survives. Not every fixer consults lint.File.GeneratedRanges
// (trailing-space trimming and blank-line collapsing, for instance, do
// not), so appending the region spans to that set is not enough on its
// own — this byte-level restore is the hard guarantee that a declared
// region is opaque to the whole fix pipeline.
//
// Regions are located in both buffers by a matched-pair marker scan and
// paired in document order. When a marker type yields a different count
// of matched pairs in the two buffers — a fixer added or removed a
// marker line — that type is left untouched, since the pairing would be
// ambiguous. Returns fixed unchanged when no marker pairs apply.
func Restore(original, fixed []byte, regions []config.ForeignRegion) []byte {
	for _, reg := range regions {
		origSpans := matchedRegionSpans(original, reg)
		fixedSpans := matchedRegionSpans(fixed, reg)
		if len(origSpans) == 0 || len(origSpans) != len(fixedSpans) {
			continue
		}
		fixed = spliceSpans(original, fixed, origSpans, fixedSpans)
	}
	return fixed
}

// spliceSpans rebuilds fixed in one pass, replacing each fixed span with
// the corresponding original span's bytes. Spans are in document order,
// so a single forward walk suffices — one allocation instead of the
// per-span full-buffer rebuild a reverse splice would cost.
func spliceSpans(original, fixed []byte, origSpans, fixedSpans []byteSpan) []byte {
	out := make([]byte, 0, len(fixed))
	prev := 0
	for i, fs := range fixedSpans {
		out = append(out, fixed[prev:fs.start]...)
		out = append(out, original[origSpans[i].start:origSpans[i].end]...)
		prev = fs.end
	}
	return append(out, fixed[prev:]...)
}

// byteSpan is a half-open [start, end) byte range within a buffer.
type byteSpan struct {
	start int
	end   int
}

// matchedRegionSpans returns one byte span per well-formed marker pair
// in src for reg — from the first byte of the start-marker line through
// the last byte of the end-marker line (excluding that line's trailing
// newline). Unmatched or orphaned markers contribute no span, matching
// Scan's range set.
func matchedRegionSpans(src []byte, reg config.ForeignRegion) []byteSpan {
	start := strings.TrimSpace(reg.Start)
	end := strings.TrimSpace(reg.End)
	var spans []byteSpan
	openStart := -1
	lineStart := 0
	n := len(src)
	for lineStart <= n {
		lineEnd := n
		next := n + 1
		if rel := bytes.IndexByte(src[lineStart:], '\n'); rel >= 0 {
			lineEnd = lineStart + rel
			next = lineEnd + 1
		}
		switch strings.TrimSpace(string(src[lineStart:lineEnd])) {
		case start:
			if openStart < 0 {
				openStart = lineStart
			}
		case end:
			if openStart >= 0 {
				spans = append(spans, byteSpan{start: openStart, end: lineEnd})
				openStart = -1
			}
		}
		lineStart = next
	}
	return spans
}
