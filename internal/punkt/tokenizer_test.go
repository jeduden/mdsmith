package punkt

import (
	"strings"
	"testing"

	sentlib "github.com/neurosnap/sentences"
	"github.com/neurosnap/sentences/english"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upstreamTokenizer returns the unmodified upstream English
// tokenizer. Used as the oracle for the fork — every Tokenize call
// against the fork must produce the same sentence slice as upstream.
func upstreamTokenizer(t *testing.T) *sentlib.DefaultSentenceTokenizer {
	t.Helper()
	tk, err := english.NewSentenceTokenizer(nil)
	require.NoError(t, err)
	return tk
}

// upstreamSentences returns the (Start, End, Text) triples upstream
// produces. Same shape as fork Tokenize for direct comparison.
func upstreamSentences(t *testing.T, text string) []Sentence {
	t.Helper()
	sents := upstreamTokenizer(t).Tokenize(text)
	out := make([]Sentence, 0, len(sents))
	for _, s := range sents {
		out = append(out, Sentence{Start: s.Start, End: s.End, Text: s.Text})
	}
	return out
}

// equivalenceCorpus exercises the cases where trained Punkt and
// naive splitting disagree — abbreviations, decimals, ellipses,
// initials, Rust-Book-style prose. It is a copy of the corpus in
// internal/mdtext/sentence_equivalence_test.go so the fork
// equivalence gate fires inside this package even before the
// integration harness in mdtext sees it.
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
	"No terminal punctuation here just a long clause that runs on",
	// CJK paragraphs — exercise the upstream-equivalent
	// IsCjkPunct boundary path and the CJK-aware HasPeriodFinal /
	// hasSentencePunct helpers. A user with Chinese/Japanese
	// Markdown enabling MDS024 must see the same sentence
	// segmentation under the fork as under upstream.
	"中文一句。中文两句。",
	"こんにちは。さようなら。",
	"中文。English mixed. More text.",
	"问题？解决方案。继续！",
}

func TestTokenize_Empty(t *testing.T) {
	tk := NewEnglish()
	assert.Nil(t, tk.Tokenize(""), "empty input must yield nil")
}

func TestTokenize_EquivalentToUpstream(t *testing.T) {
	tk := NewEnglish()
	for i, sample := range equivalenceCorpus {
		got := tk.Tokenize(sample)
		want := upstreamSentences(t, sample)
		if !assert.Equalf(t, len(want), len(got),
			"sample %d count mismatch — got %d, want %d",
			i, len(got), len(want)) {
			continue
		}
		for j := range got {
			assert.Equalf(t, want[j].Start, got[j].Start,
				"sample %d sentence %d Start", i, j)
			assert.Equalf(t, want[j].End, got[j].End,
				"sample %d sentence %d End", i, j)
			assert.Equalf(t, want[j].Text, got[j].Text,
				"sample %d sentence %d Text", i, j)
		}
	}
}

// TestTokenize_AbbreviationsMatchUpstream is the abbr-heavy slice
// equivalent — uses the same corpus shape testcorpus.AbbrHeavy
// exposes to MDS024's benchmark. Verifies the fork's segmentation
// is byte-identical to upstream on the hot dose of period-rich
// tokens that exercise multiPunctAnnotation.
func TestTokenize_AbbreviationsMatchUpstream(t *testing.T) {
	tk := NewEnglish()
	abbrCorpus := []string{
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
	for i, sample := range abbrCorpus {
		got := tk.Tokenize(sample)
		want := upstreamSentences(t, sample)
		if !assert.Equalf(t, len(want), len(got),
			"sample %d count mismatch — got %d (%v), want %d (%v)",
			i, len(got), got, len(want), want) {
			continue
		}
		for j := range got {
			assert.Equalf(t, want[j].Text, got[j].Text,
				"sample %d sentence %d Text", i, j)
		}
	}
}

// TestTokenize_CjkParagraphsMatchUpstream pins the fork's
// sentence-level output against upstream on CJK input — different
// guarantee from TestTokenize_CjkPunctuationSplitsTokens (which
// checks only word-token shape). MDS024 consumes Sentence.Text, so
// the gate here is what end users actually see when MDS024 is
// enabled against a non-English Markdown corpus.
func TestTokenize_CjkParagraphsMatchUpstream(t *testing.T) {
	tk := NewEnglish()
	corpus := []string{
		"中文一句。中文两句。中文三句。",
		"こんにちは。世界。さようなら。",
		"中文。English mixed in. 更多中文。",
		"问题？回答。继续！结束。",
		// A long-ish multi-sentence CJK paragraph that MDS024
		// might trip on if segmentation were broken.
		"产品发布前需要测试。文档需要校对。配置需要审查。最后发布。",
	}
	for i, sample := range corpus {
		got := tk.Tokenize(sample)
		want := upstreamSentences(t, sample)
		if !assert.Equalf(t, len(want), len(got),
			"CJK sample %d count mismatch — got %d (%v), want %d (%v)",
			i, len(got), got, len(want), want) {
			continue
		}
		for j := range got {
			assert.Equalf(t, want[j].Text, got[j].Text,
				"CJK sample %d sentence %d Text", i, j)
		}
	}
}

func TestTokenize_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	tk := NewEnglish()
	// Warm pool so the first allocation does not count.
	_ = tk.Tokenize("Hello world.")
	// Use one of the abbr-heavy samples; representative of the
	// MDS024 hot path.
	sample := "Dr. Smith met Mr. Jones at 3.14 p.m. on Jan. 5."
	allocs := testing.AllocsPerRun(50, func() {
		_ = tk.Tokenize(sample)
	})
	t.Logf("Tokenize allocs/op = %.1f on %d-byte abbr-heavy sample",
		allocs, len(sample))
	// The internal pipeline ideally allocates only the final result
	// slice on a warm pool. AllocsPerRun returns float; a steady
	// state of <= 4 leaves room for the result slice + small copies
	// of trimmed Text headers on growth.
	assert.LessOrEqualf(t, allocs, 4.0,
		"Tokenize must hit ≤ 4 allocs/op on a warm pool; got %.1f",
		allocs)
}

// TestTokenize_Concurrent ensures the pool is safe under concurrent
// callers. Two goroutines tokenize the same text concurrently and
// must agree on the result — the pool can't leak mutated state.
func TestTokenize_Concurrent(t *testing.T) {
	tk := NewEnglish()
	sample := "Dr. Smith met Mr. Jones at 3.14 p.m. on Jan. 5."
	want := tk.Tokenize(sample)
	const goroutines = 8
	done := make(chan []Sentence, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			done <- tk.Tokenize(sample)
		}()
	}
	for i := 0; i < goroutines; i++ {
		got := <-done
		require.Equal(t, len(want), len(got),
			"concurrent goroutine %d count mismatch", i)
		for j := range got {
			assert.Equal(t, want[j].Text, got[j].Text,
				"concurrent goroutine %d sentence %d", i, j)
		}
	}
}

// TestTokenize_LongInputDoesNotPanic exercises the buffer-growth
// branches with input larger than the initial pool capacities.
func TestTokenize_LongInputDoesNotPanic(t *testing.T) {
	tk := NewEnglish()
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("This is sentence number ")
		b.WriteString(strings.Repeat("x", i%50))
		b.WriteString(". ")
	}
	got := tk.Tokenize(b.String())
	require.NotEmpty(t, got, "long input must produce sentences")
}

func TestLoadEnglishStorage_HappyPath(t *testing.T) {
	// Drives loadEnglishStorage with valid JSON. The three supervised
	// abbreviations must end up in AbbrevTypes.
	raw := []byte(`{
        "AbbrevTypes": {"dr": 1},
        "Collocations": {},
        "SentStarters": {},
        "OrthoContext": {}
    }`)
	s := loadEnglishStorage(raw)
	require.NotNil(t, s)
	for _, abbr := range []string{"sgt", "gov", "no", "dr"} {
		assert.Truef(t, s.AbbrevTypes.Has(abbr),
			"AbbrevTypes must contain %q after loadEnglishStorage", abbr)
	}
}

func TestLoadEnglishStorage_PanicsOnMalformed(t *testing.T) {
	// Drives the error branch in loadEnglishStorage red/green.
	// Production NewEnglish would surface this same panic if the
	// bundled asset ever became corrupt.
	defer func() {
		got := recover()
		require.NotNil(t, got,
			"loadEnglishStorage must panic on malformed input")
		msg, ok := got.(string)
		require.Truef(t, ok, "panic value must be a string, got %T", got)
		assert.Contains(t, msg,
			"punkt: failed to load English training data:",
			"panic message must name the loader so the cause is "+
				"obvious without a stack walk")
	}()
	_ = loadEnglishStorage([]byte("not json"))
}

// TestTokenize_TrailingMultiByteSentenceIncludesTail proves that the
// matched word-tokenizer quirk (see
// TestTokenize_TrailingMultiByteRuneMatchesUpstream in
// word_tokenizer_test.go) does not affect user-visible Sentence
// output: the sentence emitter's trailing-fallback covers the bytes
// the word tokenizer would otherwise drop.
func TestTokenize_TrailingMultiByteSentenceIncludesTail(t *testing.T) {
	tk := NewEnglish()
	const text = "Hello 世"
	sents := tk.Tokenize(text)
	require.Len(t, sents, 1,
		"input must produce one sentence even though the word "+
			"tokenizer drops the trailing multi-byte token")
	assert.Equal(t, text, sents[0].Text,
		"the Sentence.Text must include the trailing multi-byte rune "+
			"because the sentence emitter's `text[lastBreak:]` fallback "+
			"covers any tail not marked by a SentBreak token")
}
