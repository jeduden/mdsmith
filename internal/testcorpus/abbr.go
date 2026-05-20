// Package testcorpus holds shared text corpora for tests and
// benchmarks across mdsmith's internal packages. Each corpus is a
// frozen reference input that pins behaviour or measures the cost of
// a hot code path; lifting them out of any one test package lets
// multiple call sites benchmark the same bytes.
package testcorpus

// AbbrHeavy is the abbreviation-heavy paragraph corpus that exercises
// trained Punkt's third-pass MultiPunctWordAnnotation. Every paragraph
// is built around period-rich tokens — initials, honorifics, dotted
// abbreviations, decimals, version numbers — exactly the input shape
// where the segmenter's per-token machinery is hottest. The corpus is
// imported by both mdtext's BenchmarkSplitSentences_Subset and the
// paragraph-structure rule's BenchmarkRule_MDS024 so both gates
// measure the same dose of the hot frame. See plan 193 task 1 for the
// rationale.
var AbbrHeavy = []string{
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

// AbbrHeavyParagraph joins AbbrHeavy into one long Markdown
// paragraph, matching how a real .md file looks when MDS024's
// per-paragraph segmenter runs on it. The benchmark for the rule
// uses this shape so the fixture is a single paragraph the rule
// extracts from the AST, not the slice of independent strings.
func AbbrHeavyParagraph() string {
	if len(AbbrHeavy) == 0 {
		return ""
	}
	n := 0
	for _, s := range AbbrHeavy {
		n += len(s) + 1
	}
	out := make([]byte, 0, n)
	for i, s := range AbbrHeavy {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, s...)
	}
	return string(out)
}
