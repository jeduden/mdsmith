package paragraphstructure

import (
	"strings"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// firstParagraph parses body and returns the first non-table
// paragraph's extracted text, 1-based line, and path — exactly what
// the shared collector now feeds checkParagraph in production.
func firstParagraph(t *testing.T, body string) (text string, line int, path string) {
	t.Helper()
	f, err := lint.NewFile("t.md", []byte(body+"\n"))
	require.NoError(t, err)
	paras := astutil.CollectSectionParagraphs(f)
	require.NotEmpty(t, paras, "no paragraph parsed from %q", body)
	return paras[0].ExtractText(f.Source), paras[0].Line, f.Path
}

func TestRule_checkParagraph(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}

	t.Run("guard short-circuits clean paragraph", func(t *testing.T) {
		text, line, path := firstParagraph(t, "Short and safe.")
		assert.Nil(t, r.checkParagraph(text, line, path))
	})

	t.Run("too many sentences", func(t *testing.T) {
		text, line, path := firstParagraph(t,
			"One. Two. Three. Four. Five. Six. Seven. Eight.")
		d := r.checkParagraph(text, line, path)
		require.Len(t, d, 1)
		assert.Contains(t, d[0].Message, "too many sentences")
	})

	t.Run("sentence too long", func(t *testing.T) {
		text, line, path := firstParagraph(t, strings.Repeat("word ", 45)+".")
		d := r.checkParagraph(text, line, path)
		require.Len(t, d, 1)
		assert.Contains(t, d[0].Message, "sentence too long")
	})
}

func TestCheapBounds(t *testing.T) {
	cases := []struct {
		text       string
		wantSentUB int
		wantWords  int
	}{
		{"", 1, 0},
		{"   \n  ", 1, 0},
		{"one two three", 1, 3},
		{"Hello. World!", 3, 2},
		{"e.g. this is one sentence.", 4, 5},
		{"a... b", 4, 2},
		{"q? r? s?", 4, 3},
		// CJK full-width period 。 counts toward sentUB — it IS
		// a SentBreak (via HasPeriodFinal). The CJK enders run
		// together with no whitespace, so word count is 1.
		{"一。二。三。", 4, 1},
		// Full-width ！ and ？ are NOT sentence breaks in the
		// English pipeline (HasSentEndChars is ASCII-only). The
		// guard counts only the single 。, not the ！ / ？.
		{"问题？回答。继续！", 2, 1},
		// Mixed ASCII + CJK: ASCII `.` and `!` both count, plus 。.
		{"Hello. 中文。 World!", 4, 3},
	}
	for _, c := range cases {
		ub, w := cheapBounds(c.text)
		assert.Equalf(t, c.wantSentUB, ub, "sentUB for %q", c.text)
		assert.Equalf(t, c.wantWords, w, "words for %q", c.text)
	}
}

// TestSentencePreview pins the short-and-truncated branches of the
// preview helper. The Check-path tests above only ever fire on
// sentences longer than the truncation limit (MaxWords default 40 vs
// preview limit 10), so the short branch — sentence shorter than the
// limit, returned verbatim — was uncovered in the integration
// pyramid. Both cases are pinned here as a unit.
func TestSentencePreview(t *testing.T) {
	t.Run("short sentence is returned verbatim", func(t *testing.T) {
		got := sentencePreview("just three words", 10)
		assert.Equal(t, `"just three words"`, got)
	})
	t.Run("long sentence is truncated with ellipsis", func(t *testing.T) {
		got := sentencePreview("one two three four five six seven", 3)
		assert.Equal(t, `"one two three ..."`, got)
	})
	t.Run("leading and trailing whitespace is trimmed before split", func(t *testing.T) {
		got := sentencePreview("   alpha   beta   ", 10)
		assert.Equal(t, `"alpha beta"`, got)
	})
}

// The skip guard must be sound: whenever cheapBounds is within both
// limits, the full Punkt-based Check must produce zero diagnostics.
// This pins the invariant "Punkt sentence count <= terminal-punct +
// 1 and any sentence's words <= the paragraph's words".
func TestCheapBounds_GuardIsSound(t *testing.T) {
	texts := []string{
		"Short and safe.",
		"No punctuation here just words and more words",
		"Dr. Smith met Mr. Jones at 3.14 p.m. on Jan. 5.",
		"One. Two. Three. Four. Five.",
		strings.Repeat("word ", 39) + "end.",
		"Ellipses... and more... still going... but short.",
		// Full-width ！ and ？ are NOT sentence breaks in the
		// English pipeline, so a paragraph that uses only them
		// between clauses segments as ONE sentence. The guard
		// must NOT count them as terminal punctuation — counting
		// would still be sound (UB stays an upper bound) but the
		// tighter bound below pins the actual segmenter behaviour.
		"问题？回答！继续？",
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	for _, txt := range texts {
		ub, w := cheapBounds(txt)
		if ub <= r.MaxSentences && w <= r.MaxWords {
			diags := r.Check(mustParaFile(t, txt))
			assert.Emptyf(t, diags, "guard passed but Check flagged %q: %v", txt, diags)
		}
	}
}

// TestSentBufPool_ClearReleasesStringReferences pins the contract
// behind the `clear(sentences)` line in checkParagraph: a
// `sync.Pool`-stored slice retains its backing array indefinitely,
// and the GC scans the array across its full capacity (not just up
// to len). Truncating with `[:0]` therefore leaves prior string
// elements reachable through the pool until they are overwritten or
// the pool entry is evicted by a GC sweep. `clear()` zeros every
// slot so the pool only holds nil string headers.
//
// The test demonstrates both halves: without clear, a prior string
// survives the truncate-and-Put round-trip; with clear, it is gone.
// A regression that drops the clear() in checkParagraph would
// silently re-introduce the retention.
func TestSentBufPool_ClearReleasesStringReferences(t *testing.T) {
	var p sync.Pool
	p.New = func() any {
		s := make([]string, 0, 4)
		return &s
	}

	t.Run("without clear: prior string survives [:0]+Put", func(t *testing.T) {
		ptr := p.Get().(*[]string)
		*ptr = append(*ptr, "pinned-MARKER-1234")
		*ptr = (*ptr)[:0] // truncate without clear
		p.Put(ptr)

		next := p.Get().(*[]string)
		require.GreaterOrEqual(t, cap(*next), 4)
		underlying := (*next)[:cap(*next)]
		assert.Equal(t, "pinned-MARKER-1234", underlying[0],
			"truncated slice keeps the prior string at index 0 — "+
				"the GC sees it through the pool's holder")
		// Restore the pool to clean state for the next subtest.
		clear(underlying)
	})

	t.Run("with clear: prior string is gone", func(t *testing.T) {
		ptr := p.Get().(*[]string)
		*ptr = append(*ptr, "would-be-pinned-MARKER")
		clear(*ptr)
		*ptr = (*ptr)[:0]
		p.Put(ptr)

		next := p.Get().(*[]string)
		require.GreaterOrEqual(t, cap(*next), 4)
		underlying := (*next)[:cap(*next)]
		assert.Equal(t, "", underlying[0],
			"clear() zeroes the slot — the prior string can no "+
				"longer keep its referent alive through the pool")
	})
}

// TestCheapBounds_FullWidthExclamQuestionNotSentBreaks pins the
// invariant cheapBounds relies on: in the English Punkt pipeline
// (the only one mdtext.SplitSentences runs), full-width ！ and ？
// are word boundaries but NOT sentence boundaries. The cheapBounds
// rune set excludes them on this basis; the test would have failed
// red against an implementation that emitted SentBreak for either
// rune, prompting a fix to keep the set aligned.
func TestCheapBounds_FullWidthExclamQuestionNotSentBreaks(t *testing.T) {
	for _, text := range []string{
		"中文！更多",
		"中文？更多",
		"问题？回答！继续？",
	} {
		got := mdtext.SplitSentences(text)
		require.Lenf(t, got, 1,
			"text %q must segment as ONE sentence in the English "+
				"pipeline because ！ / ？ are not sentence-break "+
				"runes; got %v", text, got)
	}
}

func mustParaFile(t *testing.T, body string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("t.md", []byte(body+"\n"))
	require.NoError(t, err)
	return f
}

func TestCheck_TooManySentences(t *testing.T) {
	// 8 sentences, default max is 6.
	src := []byte("One. Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	d := diags[0]
	if d.RuleID != "MDS024" {
		t.Errorf("expected rule ID MDS024, got %s", d.RuleID)
	}
	assert.Contains(t, d.Message, "too many sentences", "unexpected message: %s", d.Message)
	assert.Contains(t, d.Message, "8 > 6", "expected count in message, got: %s", d.Message)
}

func TestCheck_UnderSentenceLimit(t *testing.T) {
	src := []byte("One. Two. Three.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestCheck_SentenceTooLong(t *testing.T) {
	// Build a sentence with 45 words.
	words := make([]string, 45)
	for i := range words {
		words[i] = "word"
	}
	src := []byte(strings.Join(words, " ") + ".\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	assert.Contains(t, diags[0].Message, "sentence too long", "unexpected message: %s", diags[0].Message)
	assert.Contains(t, diags[0].Message, "45 > 40", "expected count in message, got: %s", diags[0].Message)
	assert.Contains(t, diags[0].Message, "word word word word word",
		"expected sentence preview in message, got: %s", diags[0].Message)
}

func TestCheck_SentenceTooLong_ShowsPreview(t *testing.T) {
	src := []byte("The quick brown fox jumped over the lazy dog " +
		"and kept running through the meadow until it reached " +
		"the very end of the long winding road that stretched " +
		"far beyond the hills. Short.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 10, MaxWords: 10}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	// Should show first ~10 words of the offending sentence as preview.
	assert.Contains(t, diags[0].Message, "\"The quick brown fox jumped over the lazy dog and ...\"",
		"expected truncated preview, got: %s", diags[0].Message)
}

func TestCheck_BothLimitsExceeded(t *testing.T) {
	// 8 sentences (exceeds max 6) and one sentence with 45 words (exceeds max 40).
	words := make([]string, 45)
	for i := range words {
		words[i] = "word"
	}
	longSent := strings.Join(words, " ") + "."
	src := []byte(longSent + " Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d: %v", len(diags), diags)
	hasSentences := false
	hasWords := false
	for _, d := range diags {
		if strings.Contains(d.Message, "too many sentences") {
			hasSentences = true
		}
		if strings.Contains(d.Message, "sentence too long") {
			hasWords = true
		}
	}
	assert.True(t, hasSentences, "missing 'too many sentences' diagnostic")
	assert.True(t, hasWords, "missing 'sentence too long' diagnostic")
}

func TestCheck_CustomSettings(t *testing.T) {
	src := []byte("One. Two. Three. Four.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 3, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	assert.Contains(t, diags[0].Message, "4 > 3", "expected custom limit in message, got: %s", diags[0].Message)
}

func TestCheck_ShortParagraph(t *testing.T) {
	src := []byte("Short.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestCheck_DiagnosticFields(t *testing.T) {
	src := []byte("One. Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if d.RuleName != "paragraph-structure" {
		t.Errorf("expected rule name paragraph-structure, got %s", d.RuleName)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
}

func TestCheck_TableSkipped(t *testing.T) {
	// A markdown table parsed as a paragraph should be skipped.
	src := []byte("| A | B | C | D | E | F | G | H |\n" +
		"|---|---|---|---|---|---|---|---|\n" +
		"| one | two | three | four | five | six | seven | eight |\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 1, MaxWords: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for table, got %d", len(diags))
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{
		"max-sentences":          10,
		"max-words-per-sentence": 50,
	})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.MaxSentences != 10 {
		t.Errorf("expected MaxSentences=10, got %d", r.MaxSentences)
	}
	if r.MaxWords != 50 {
		t.Errorf("expected MaxWords=50, got %d", r.MaxWords)
	}
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"max-sentences": "not-a-number"})
	require.Error(t, err, "expected error for non-int max-sentences")
}

func TestApplySettings_InvalidMaxWordsType(t *testing.T) {
	// The max-words-per-sentence branch had no negative-path test
	// despite mirroring max-sentences; without it a regression in
	// the type-check would only show on a real config error from a
	// user, not in CI.
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"max-words-per-sentence": "not-a-number"})
	require.Error(t, err, "expected error for non-int max-words-per-sentence")
	assert.Contains(t, err.Error(), "max-words-per-sentence")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	require.Error(t, err, "expected error for unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max-sentences"] != 6 {
		t.Errorf("expected max-sentences=6, got %v", ds["max-sentences"])
	}
	if ds["max-words-per-sentence"] != 40 {
		t.Errorf("expected max-words-per-sentence=40, got %v", ds["max-words-per-sentence"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS024" {
		t.Errorf("expected MDS024, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "paragraph-structure" {
		t.Errorf("expected paragraph-structure, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "prose" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}

// --- Placeholder tests ---

func TestCheck_Placeholder_VarToken_MaskedToWord(t *testing.T) {
	// A paragraph consisting only of a var-token placeholder is masked to
	// "word" (one word, no punctuation), well below max-sentences and max-words,
	// so no diagnostic is produced.
	src := []byte("# Title\n\n{body}\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40, Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	require.Empty(t, diags, "var-token paragraph masked to neutral word should produce no diagnostic")
}

func TestCheck_Placeholder_EmptyList_StructureChecksRun(t *testing.T) {
	// Without placeholders configured, behavior is unchanged.
	// A paragraph with many sentences still gets flagged.
	src := []byte("# Title\n\nFirst. Second. Third. Fourth. Fifth. Sixth. Seventh.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40, Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "over-sentence paragraph should still be flagged without placeholders")
}

func TestApplySettings_Placeholders_ParagraphStructure(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"var-token"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"var-token"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken_ParagraphStructure(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"placeholders": []any{"bad"}})
	require.Error(t, err)
}

func TestApplySettings_Placeholders_NonList_ParagraphStructure(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of strings")
}

func TestDefaultSettings_ParagraphStructure_IncludesPlaceholders(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_ParagraphStructure(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("max-sentences"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}

// TestEnabledByDefault pins MDS024 as opt-in. Punkt-based exact
// sentence counting and per-sentence word counting cost ~20% of
// mdsmith's wall time on prose-heavy input (plan 187 profile); the
// rule's value is the precise diagnostic, so users who want it
// enable it explicitly rather than every default check paying the
// segmenter cost.
func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault(),
		"MDS024 must be opt-in — see EnabledByDefault godoc for cost rationale")
}
