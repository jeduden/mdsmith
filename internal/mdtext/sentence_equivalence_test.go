package mdtext_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plan 187 task 4: sentence-segmenter equivalence harness.
//
// SplitSentences wraps the trained Punkt tokenizer
// (github.com/neurosnap/sentences). It is the single biggest
// neutral-corpus cost (~20% CPU, heavy regexp backtracking). MDS024
// (paragraph-structure) is its only production caller and its
// diagnostics depend on the EXACT segmentation: the sentence count,
// each sentence's word count, and the over-long-sentence preview
// string. So a faster segmenter is only adoptable if it is
// byte-for-byte identical to Punkt on real prose.
//
// This harness makes that check reusable: assertSegmenterEquivalent
// runs a candidate against Punkt over a representative corpus and
// fails on the first divergence. A future faster candidate is
// adopted only if it passes here.
//
// Evidence-backed negative recorded for this task (see plan 187):
//
//   - The "per-call regexp.MustCompile in IsNonPunct" hypothesis is
//     false. IsNonPunct has no call site anywhere in the neurosnap
//     module or its english subpackage, and no regexp.MustCompile
//     frame appears in the neutral CPU profile. Precompiling it
//     would change nothing.
//   - The real cost is intrinsic trained-Punkt regexp *execution*:
//     english.(*MultiPunctWordAnnotation).tokenAnnotation (~140 ms
//     cum on the neutral corpus) running package-level regexps
//     (reAbbr and the token-type matchers) with backtracking. That
//     is the trained model's algorithm, not a fixable recompile.
//   - A naive [.!?] splitter is provably NOT equivalent (TestNaive*
//     below): it diverges on abbreviations, decimals, ellipses, and
//     initials — exactly what Punkt is trained to handle. No pure-Go
//     Punkt-compatible faster segmenter exists.
//
// Conclusion: keep Punkt. The neutral lever is the structural one
// (plan 187 task 3 / 5), not a segmenter swap. This harness stays as
// the cheap gate for any future candidate.

// equivalenceCorpus is representative prose that exercises the cases
// where trained Punkt and naive splitting disagree: plain
// multi-sentence text, abbreviations, honorifics, decimals,
// ellipses, initials, and Rust-Book-style technical prose (the
// neutral benchmark corpus' shape).
var equivalenceCorpus = []string{
	"Hello world. How are you? I am fine!",
	"Dr. Smith met Mr. Jones at 3.14 p.m. on Jan. 5.",
	"The value is 3.14 today. It was 2.71 yesterday.",
	"Wait... what happened here? Nothing, apparently.",
	"J. R. R. Tolkien wrote it. Many people read it.",
	"Use e.g. this form, i.e. the short one. Then stop.",
	"The U.S. and U.K. signed it. The E.U. did not.",
	"A trait bound restricts the generic types a function accepts. " +
		"It is written after a colon. The compiler enforces it at " +
		"every call site. Errors point at the unsatisfied bound.",
	"Ownership moves by default. Borrowing lends a reference instead. " +
		"A mutable borrow is exclusive. The borrow checker proves this " +
		"at compile time, so no runtime cost is added.",
	"See section 1.2.3 for details. The API is stable. Version 2.0 " +
		"dropped the old path. Migrate before then.",
	"",
	"No terminal punctuation here just a long clause that runs on",
}

// firstDivergence returns a human-readable detail of the first corpus
// sample where candidate's segmentation differs from Punkt's, or
// ok==true when the candidate is byte-for-byte equivalent across the
// whole corpus. The detail embeds the corpus index, so callers do not
// need it separately. This is the reusable gate any future faster
// candidate must pass (ok==true) to be adoptable.
func firstDivergence(candidate func(string) []string) (detail string, ok bool) {
	for i, sample := range equivalenceCorpus {
		want := mdtext.SplitSentences(sample)
		got := candidate(sample)
		if !slicesEqual(want, got) {
			return fmt.Sprintf(
				"corpus[%d]=%q\n  Punkt:     %#v\n  candidate: %#v",
				i, sample, want, got), false
		}
	}
	return "", true
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertSegmenterEquivalent fails the test unless candidate is
// byte-for-byte equivalent to Punkt across the whole corpus.
func assertSegmenterEquivalent(
	t *testing.T, name string, candidate func(string) []string,
) {
	t.Helper()
	detail, ok := firstDivergence(candidate)
	require.Truef(t, ok,
		"%s must be byte-for-byte equivalent to Punkt; diverged at %s",
		name, detail)
}

// TestSplitSentences_IsItsOwnReference sanity-checks the harness:
// Punkt is trivially equivalent to itself across the whole corpus,
// so a real candidate that passes is genuinely byte-identical.
func TestSplitSentences_IsItsOwnReference(t *testing.T) {
	assertSegmenterEquivalent(t, "Punkt", mdtext.SplitSentences)
}

// naiveSplit is the obvious "fast" alternative: split on a terminal
// punctuation run followed by whitespace or end of text, trim, drop
// empties — mirroring SplitSentences' post-processing so only the
// boundary logic differs.
var naiveBoundary = regexp.MustCompile(`[.!?]+(\s+|$)`)

func naiveSplit(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var out []string
	start := 0
	for _, m := range naiveBoundary.FindAllStringIndex(text, -1) {
		seg := strings.TrimSpace(text[start:m[1]])
		if seg != "" {
			out = append(out, seg)
		}
		start = m[1]
	}
	if start < len(text) {
		if seg := strings.TrimSpace(text[start:]); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// TestNaiveSplit_IsNotPunktEquivalent records the negative with a
// concrete divergence: naive splitting breaks "Dr. Smith met Mr.
// Jones at 3.14 p.m. on Jan. 5." into many fragments where trained
// Punkt keeps one sentence. This is why MDS024 needs Punkt and why
// no naive faster segmenter is adoptable.
func TestNaiveSplit_IsNotPunktEquivalent(t *testing.T) {
	const sample = "Dr. Smith met Mr. Jones at 3.14 p.m. on Jan. 5."

	punkt := mdtext.SplitSentences(sample)
	naive := naiveSplit(sample)

	require.Len(t, punkt, 1,
		"trained Punkt keeps the abbreviation-laden clause as one sentence")
	assert.Greater(t, len(naive), len(punkt),
		"naive splitting over-segments on abbreviations and decimals")

	// And it fails the reusable gate, so it cannot be adopted: the
	// harness must reject any non-Punkt-equivalent candidate.
	detail, ok := firstDivergence(naiveSplit)
	assert.Falsef(t, ok,
		"the equivalence harness must reject naiveSplit, but it passed")
	t.Logf("recorded negative — naiveSplit diverges from Punkt at %s", detail)
}

// BenchmarkSplitSentences pins the Punkt cost over the equivalence
// corpus so any future candidate has a comparison baseline and a
// regression in the tokenizer path is visible.
func BenchmarkSplitSentences(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, s := range equivalenceCorpus {
			_ = mdtext.SplitSentences(s)
		}
	}
}

// abbrHeavyCorpus is the abbreviation-heavy slice that exercises the
// reAbbr-driven path inside MultiPunctWordAnnotation. Every paragraph
// is built around period-rich tokens — initials, honorifics, dotted
// abbreviations, decimals, version numbers — exactly the input shape
// where the regex's backtracking loop is hottest. Compared with the
// full equivalence corpus, this is a 5–10x denser dose of the hot
// frame, so an optimization on that frame is visible here without
// being diluted by ordinary prose. See plan 191 task 1.
var abbrHeavyCorpus = []string{
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

// BenchmarkSplitSentences_Subset measures Punkt's wall time on
// abbreviation-heavy prose. The MultiPunctWordAnnotation third
// pass fires once per period-ending token; this corpus is the
// densest such input. Under the default build it exercises
// matchAbbrPattern inside fastMultiPunctWordAnnotation; under
// `-tags mdtext_punkt_upstream` it exercises reAbbr inside
// english.MultiPunctWordAnnotation. The plan 191 acceptance bar
// is a ≥10% improvement of the default build over the upstream
// build here. The full BenchmarkSplitSentences number remains
// the equivalence-corpus baseline; this one isolates the lever.
func BenchmarkSplitSentences_Subset(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, s := range abbrHeavyCorpus {
			_ = mdtext.SplitSentences(s)
		}
	}
}
