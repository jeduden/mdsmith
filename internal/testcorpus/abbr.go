// Package testcorpus holds shared text corpora for tests and
// benchmarks across mdsmith's internal packages. Each corpus is a
// frozen reference input that pins behaviour or measures the cost of
// a hot code path; lifting them out of any one test package lets
// multiple call sites benchmark the same bytes.
package testcorpus

import (
	"slices"
	"strings"
)

// abbrHeavy is the abbreviation-heavy paragraph corpus that
// exercises trained Punkt's third-pass MultiPunctWordAnnotation.
// Every paragraph is built around period-rich tokens — initials,
// honorifics, dotted abbreviations, decimals, version numbers —
// exactly the input shape where the segmenter's per-token machinery
// is hottest. Kept unexported so the canonical bytes cannot be
// mutated by callers (a slice assignment in one test would silently
// skew every other consumer running in parallel). See plan 193
// task 1 for the rationale and AbbrHeavy / AbbrHeavyParagraph for
// the public read-only accessors.
var abbrHeavy = []string{
	"Dr. Smith met Mr. Jones at 3.14 p.m. on Jan. 5. " +
		"Mrs. Lee then arrived at 4.30 p.m. with Ms. Park.",
	"The U.S. and U.K. signed it at 10.30 a.m. " +
		"The E.U. and U.S.S.R. did not at 11.45 a.m.",
	"J. R. R. Tolkien wrote it. C. S. Lewis read it. " +
		"T. S. Eliot reviewed it. W. B. Yeats praised it.",
	"Use e.g. this short form, i.e. the abbreviated one, " +
		"vs. the long form, etc. See sec. 1.2.3 of the doc.",
	"At No. 1026.253.553, the F.B.I. arrived at 7.15 a.m. " +
		"The C.I.A. and N.S.A. followed at 8.30 a.m.",
	"Version 1.2.3 dropped Mr. Smith's API at v. 2.0. " +
		"See appendix A.1.2 vs. appendix B.3.4 for details.",
	"He worked for the U.S. govt. from Jan. 1990 to Dec. 2005. " +
		"She worked for the U.K. govt. from Feb. 1995 to Nov. 2010.",
	"Prof. Adams cited Smith et al., 2020, p. 14, sec. 2.3. " +
		"Dr. Brown cited Jones et al., 2021, p. 22, sec. 4.5.",
}

// AbbrHeavy returns a copy of the abbreviation-heavy paragraph
// corpus. Each call allocates a fresh []string so callers cannot
// mutate the canonical fixture; the strings themselves are
// immutable in Go, so this fully isolates consumers from each other.
// The corpus is imported by both mdtext's BenchmarkSplitSentences_Subset
// and the paragraph-structure rule's BenchmarkRule_MDS024 so both
// gates measure the same bytes — but with no shared mutable state.
func AbbrHeavy() []string { return slices.Clone(abbrHeavy) }

// AbbrHeavyParagraph joins the abbreviation-heavy corpus into one
// long Markdown paragraph, matching how a real .md file looks when
// MDS024's per-paragraph segmenter runs on it. The benchmark for the
// rule uses this shape so the fixture is a single paragraph the
// rule extracts from the AST, not the slice of independent strings.
//
// The join logic itself is in joinWithSpace so the empty-slice
// branch can be tested without touching the package-level corpus.
func AbbrHeavyParagraph() string {
	return joinWithSpace(abbrHeavy)
}

// joinWithSpace concatenates the elements of items with a single
// space separator. Returns "" for nil/empty input.
//
// Uses strings.Builder with a pre-sized Grow so the joined result
// lives in a single backing array — and strings.Builder.String()
// hands that array back without a copy. Grow's argument matches the
// final length exactly (sum(len(s) for s in items) plus
// len(items)-1 for the single-space separators).
func joinWithSpace(items []string) string {
	if len(items) == 0 {
		return ""
	}
	n := len(items) - 1
	for _, s := range items {
		n += len(s)
	}
	var b strings.Builder
	b.Grow(n)
	for i, s := range items {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	return b.String()
}
