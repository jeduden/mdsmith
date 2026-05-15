package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// RenderHint returns a best-effort suggestion for fixing the
// violation. It only fires on a small set of shapes:
//
//   - String disjunctions: when the actual value is within
//     Levenshtein distance 2 of one of the literal alternatives,
//     suggest that literal.
//   - Integer ranges: when the actual value is just outside the
//     declared bounds, suggest the nearest bound.
//
// All other shapes return the empty string. A noisy hint (for
// instance, a regex pattern naïvely echoed at the user) is worse
// than no hint, so the extractor errs on the side of silence.
//
// actual is the raw front-matter value as decoded from JSON;
// numbers arrive as float64 from json.Unmarshal.
func RenderHint(expr string, actual any) string {
	expr = strings.TrimSpace(expr)
	if h := hintForStringDisjunction(expr, actual); h != "" {
		return h
	}
	if h := hintForIntRange(expr, actual); h != "" {
		return h
	}
	return ""
}

func hintForStringDisjunction(expr string, actual any) string {
	s, ok := actual.(string)
	if !ok {
		return ""
	}
	parts := splitTopLevel(expr, '|')
	if len(parts) < 2 {
		return ""
	}
	literals := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		t = strings.TrimPrefix(t, "*")
		if !isQuotedString(t) {
			return ""
		}
		unq, err := strconv.Unquote(t)
		if err != nil {
			return ""
		}
		literals = append(literals, unq)
	}
	best := ""
	bestDist := -1
	for _, lit := range literals {
		d := levenshtein(s, lit)
		if d == 0 {
			// Exact match — should not happen on a violation, but
			// returning a hint that says "did you mean X?" when X
			// equals the actual value would be confusing.
			return ""
		}
		if d > 2 {
			continue
		}
		if bestDist == -1 || d < bestDist {
			best = lit
			bestDist = d
		}
	}
	if best == "" {
		return ""
	}
	// %q safely quotes the candidate so a literal containing a
	// double quote cannot break out of the surrounding quotes in
	// the rendered message (CodeQL go/unsafe-quoting).
	return fmt.Sprintf("did you mean %q?", best)
}

func hintForIntRange(expr string, actual any) string {
	rendered, ok := renderIntRange(expr)
	if !ok {
		return ""
	}
	n, ok := toFloat64(actual)
	if !ok {
		return ""
	}
	// Re-derive the bounds from the rendered form: parseBounds
	// extracts (lo, hi) from "int between N and M" / "int >= N" /
	// "int <= N". This keeps the bound source in one place
	// (renderIntRange) and avoids duplicating the parser here.
	lo, hi, hasLo, hasHi := parseRenderedBounds(rendered)
	switch {
	case hasLo && n < float64(lo):
		return "try " + strconv.Itoa(lo)
	case hasHi && n > float64(hi):
		return "try " + strconv.Itoa(hi)
	}
	return ""
}

func parseRenderedBounds(rendered string) (lo, hi int, hasLo, hasHi bool) {
	switch {
	case strings.HasPrefix(rendered, "int between "):
		body := strings.TrimPrefix(rendered, "int between ")
		idx := strings.Index(body, " and ")
		if idx < 0 {
			return 0, 0, false, false
		}
		var err error
		lo, err = strconv.Atoi(body[:idx])
		if err != nil {
			return 0, 0, false, false
		}
		hi, err = strconv.Atoi(body[idx+len(" and "):])
		if err != nil {
			return 0, 0, false, false
		}
		return lo, hi, true, true
	case strings.HasPrefix(rendered, "int >= "):
		n, err := strconv.Atoi(strings.TrimPrefix(rendered, "int >= "))
		if err != nil {
			return 0, 0, false, false
		}
		return n, 0, true, false
	case strings.HasPrefix(rendered, "int <= "):
		n, err := strconv.Atoi(strings.TrimPrefix(rendered, "int <= "))
		if err != nil {
			return 0, 0, false, false
		}
		return 0, n, false, true
	}
	return 0, 0, false, false
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// levenshtein returns the edit distance between a and b. The
// implementation is the textbook two-row DP; the hint extractor
// only calls it on short strings (front-matter enum literals),
// so the O(len(a)*len(b)) cost is negligible. Unicode is handled
// by comparing runes, not bytes, so a typo in a multi-byte rune
// still counts as one edit.
//
// Inputs longer than maxLevInput runes short-circuit to a
// length-based upper bound. The hint extractor only uses
// distance ≤ 2 to suggest a literal, so any value beyond the
// guard is far too dissimilar to produce a hint anyway. The cap
// also satisfies CodeQL's go/allocation-size-overflow finding
// for the `len(rb)+1` row allocations: bounded inputs cannot
// overflow `int` arithmetic.
//
// The length guard runs before allocating rune slices for a or
// b: a multi-megabyte front-matter string would otherwise
// materialise into a rune slice the size of the input before we
// got a chance to short-circuit, defeating the protection the
// guard is meant to provide.
const maxLevInput = 1024

func levenshtein(a, b string) int {
	if tooLongForLevInput(a) || tooLongForLevInput(b) {
		// At least one operand exceeds the cap; return the
		// longer rune count capped at maxLevInput+1. The capped
		// value is still an upper bound on the true distance,
		// and that's all callers need — the hint extractor only
		// suggests literals within distance 2, so anything >= 3
		// (the cap is much larger) already kills the hint. The
		// `for range` walk inside runeCountAtMost iterates runes
		// without allocating, so a multi-megabyte operand never
		// materialises into a `[]rune` here.
		ca := runeCountAtMost(a, maxLevInput+1)
		cb := runeCountAtMost(b, maxLevInput+1)
		if ca > cb {
			return ca
		}
		return cb
	}
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	// CodeQL flagged the previous `make([]int, len(rb)+1)`
	// allocation because the static analyser couldn't follow
	// the maxLevInput bound through the tooLongForLevInput
	// helper above. Use the compile-time constant size
	// directly so the analyser sees a bounded allocation, and
	// only iterate the columns inside the bound.
	const rows = maxLevInput + 1
	var prev, curr [rows]int
	for j := 0; j <= len(rb); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = minInt(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func minInt(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// tooLongForLevInput reports whether s has more than maxLevInput
// runes. It iterates via `for range` so no `[]rune` slice is
// allocated and returns as soon as the threshold is crossed —
// the caller only needs the yes/no answer.
func tooLongForLevInput(s string) bool {
	n := 0
	for range s {
		n++
		if n > maxLevInput {
			return true
		}
	}
	return false
}

// runeCountAtMost returns the rune count of s capped at limit.
// Used in the levenshtein over-cap branch to compute the length
// upper bound without materialising the full rune slice for very
// long inputs.
func runeCountAtMost(s string, limit int) int {
	n := 0
	for range s {
		if n >= limit {
			return n
		}
		n++
	}
	return n
}
