package classifier

import (
	"math"
	"testing"
)

const tol = 1e-6

func floatEq(a, b float64) bool {
	return math.Abs(a-b) < tol
}

// --- CompressionRatio (bigram repetition) ---

func TestCompressionRatio_Empty(t *testing.T) {
	if got := CompressionRatio(nil); got != 0.0 {
		t.Fatalf("expected 0.0 for nil tokens, got %v", got)
	}
}

func TestCompressionRatio_SingleWord(t *testing.T) {
	if got := CompressionRatio([]string{"hello"}); got != 0.0 {
		t.Fatalf("expected 0.0 for single word, got %v", got)
	}
}

func TestCompressionRatio_RedundantVsVaried(t *testing.T) {
	redundant := []string{"the", "the", "the", "the", "the", "the", "the", "the"}
	varied := []string{"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dogs"}

	ratioRedundant := CompressionRatio(redundant)
	ratioVaried := CompressionRatio(varied)

	if ratioRedundant <= 0.5 {
		t.Fatalf("expected redundant ratio > 0.5, got %.4f", ratioRedundant)
	}
	if ratioVaried != 0.0 {
		t.Fatalf("expected varied ratio = 0, got %.4f", ratioVaried)
	}
	if ratioRedundant <= ratioVaried {
		t.Fatalf(
			"expected redundant > varied, got %.4f <= %.4f",
			ratioRedundant, ratioVaried,
		)
	}
}

func TestCompressionRatio_NoRepetition(t *testing.T) {
	r := CompressionRatio([]string{"hello", "world", "this", "is", "a", "test"})
	if r != 0.0 {
		t.Fatalf("expected 0.0 for no repeated bigrams, got %v", r)
	}
}

// --- TypeTokenRatio ---

func TestTypeTokenRatio_Empty(t *testing.T) {
	if got := TypeTokenRatio(nil); got != 0.0 {
		t.Fatalf("expected 0.0 for nil slice, got %v", got)
	}
	if got := TypeTokenRatio([]string{}); got != 0.0 {
		t.Fatalf("expected 0.0 for empty slice, got %v", got)
	}
}

func TestTypeTokenRatio_AllUnique(t *testing.T) {
	tokens := []string{"alpha", "beta", "gamma", "delta"}
	if got := TypeTokenRatio(tokens); !floatEq(got, 1.0) {
		t.Fatalf("expected 1.0 for all-unique tokens, got %v", got)
	}
}

func TestTypeTokenRatio_AllSame(t *testing.T) {
	tokens := []string{"the", "the", "the", "the"}
	want := 1.0 / 4.0
	if got := TypeTokenRatio(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func TestTypeTokenRatio_Mixed(t *testing.T) {
	// 3 unique out of 5 tokens
	tokens := []string{"a", "b", "a", "c", "b"}
	want := 3.0 / 5.0
	if got := TypeTokenRatio(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

// --- NominalDensity ---

func TestNominalDensity_Empty(t *testing.T) {
	if got := NominalDensity(nil); got != 0.0 {
		t.Fatalf("expected 0.0, got %v", got)
	}
}

func TestNominalDensity_AllNominal(t *testing.T) {
	tokens := []string{"implementation", "management", "happiness"}
	if got := NominalDensity(tokens); !floatEq(got, 1.0) {
		t.Fatalf("expected 1.0, got %v", got)
	}
}

func TestNominalDensity_NoNominal(t *testing.T) {
	tokens := []string{"run", "test", "build"}
	if got := NominalDensity(tokens); !floatEq(got, 0.0) {
		t.Fatalf("expected 0.0, got %v", got)
	}
}

func TestNominalDensity_Mixed(t *testing.T) {
	// 2 nominals out of 4
	tokens := []string{"action", "speed", "performance", "run"}
	// "action" ends in -tion, "performance" ends in -ance
	want := 2.0 / 4.0
	if got := NominalDensity(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func TestNominalDensity_EachSuffix(t *testing.T) {
	cases := []struct {
		token string
	}{
		{"description"},  // -tion
		{"development"},  // -ment
		{"correctness"},  // -ness
		{"creativity"},   // -ity
		{"performance"},  // -ance
		{"intelligence"}, // -ence
	}
	for _, c := range cases {
		got := NominalDensity([]string{c.token})
		if !floatEq(got, 1.0) {
			t.Errorf("token %q: expected NominalDensity=1.0, got %v", c.token, got)
		}
	}
}

// --- SentLenVariance ---

func TestSentLenVariance_Empty(t *testing.T) {
	if got := SentLenVariance(""); got != 0.0 {
		t.Fatalf("expected 0.0 for empty string, got %v", got)
	}
}

func TestSentLenVariance_SingleSentence(t *testing.T) {
	if got := SentLenVariance("This is a single sentence with no terminator"); got != 0.0 {
		t.Fatalf("expected 0.0 for single sentence, got %v", got)
	}
}

func TestSentLenVariance_EqualLengthSentences(t *testing.T) {
	// Two sentences of equal word count → CV = 0
	text := "one two three. four five six."
	if got := SentLenVariance(text); !floatEq(got, 0.0) {
		t.Fatalf("expected 0.0 for equal-length sentences, got %v", got)
	}
}

func TestSentLenVariance_UnequalLengthSentences(t *testing.T) {
	// Unequal sentences → CV > 0
	text := "one. two three four five six seven eight nine ten."
	got := SentLenVariance(text)
	if got <= 0.0 {
		t.Fatalf("expected positive variance for unequal sentences, got %v", got)
	}
}

func TestSentLenVariance_FiltersEmptySegments(t *testing.T) {
	// Multiple punctuation marks may produce empty segments; should be filtered.
	text := "Hello world!? This is fine."
	got := SentLenVariance(text)
	// Should not panic; result can be 0 or positive.
	if got < 0.0 {
		t.Fatalf("expected non-negative variance, got %v", got)
	}
}

// --- FuncWordRatio ---

func TestFuncWordRatio_Empty(t *testing.T) {
	if got := FuncWordRatio(nil); got != 0.0 {
		t.Fatalf("expected 0.0, got %v", got)
	}
}

func TestFuncWordRatio_AllFuncWords(t *testing.T) {
	tokens := []string{"the", "a", "and", "in", "of"}
	if got := FuncWordRatio(tokens); !floatEq(got, 1.0) {
		t.Fatalf("expected 1.0, got %v", got)
	}
}

func TestFuncWordRatio_NoFuncWords(t *testing.T) {
	tokens := []string{"implement", "classifier", "feature", "extractor"}
	if got := FuncWordRatio(tokens); !floatEq(got, 0.0) {
		t.Fatalf("expected 0.0, got %v", got)
	}
}

func TestFuncWordRatio_Mixed(t *testing.T) {
	// 2 func words out of 4
	tokens := []string{"the", "quick", "brown", "fox"}
	want := 1.0 / 4.0
	if got := FuncWordRatio(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func TestFuncWordRatio_Pronouns(t *testing.T) {
	tokens := []string{"he", "she", "they", "implement"}
	want := 3.0 / 4.0
	if got := FuncWordRatio(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

// --- AvgWordLength ---

func TestAvgWordLength_Empty(t *testing.T) {
	if got := AvgWordLength(nil); got != 0.0 {
		t.Fatalf("expected 0.0, got %v", got)
	}
}

func TestAvgWordLength_Basic(t *testing.T) {
	// "go" = 2, "rust" = 4 → mean = 3.0
	tokens := []string{"go", "rust"}
	want := 3.0
	if got := AvgWordLength(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func TestAvgWordLength_SingleToken(t *testing.T) {
	tokens := []string{"hello"}
	want := 5.0
	if got := AvgWordLength(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func TestAvgWordLength_MultipleTokens(t *testing.T) {
	// "a"=1, "bb"=2, "ccc"=3 → mean = 2.0
	tokens := []string{"a", "bb", "ccc"}
	want := 2.0
	if got := AvgWordLength(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

// --- LyAdverbDensity ---

func TestLyAdverbDensity_Empty(t *testing.T) {
	if got := LyAdverbDensity(nil); got != 0.0 {
		t.Fatalf("expected 0.0, got %v", got)
	}
}

func TestLyAdverbDensity_Basic(t *testing.T) {
	// 2 adverbs out of 3 tokens
	tokens := []string{"quickly", "slowly", "test"}
	want := 2.0 / 3.0
	if got := LyAdverbDensity(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func TestLyAdverbDensity_TooShort(t *testing.T) {
	// "fly" is only 3 chars, len < 4 → excluded
	tokens := []string{"fly"}
	if got := LyAdverbDensity(tokens); got != 0.0 {
		t.Fatalf("expected 0.0 for 'fly' (len<4), got %v", got)
	}
}

func TestLyAdverbDensity_ShortlyIncluded(t *testing.T) {
	// "ply" is 3 chars → excluded; "only" is 4 chars → included
	tokens := []string{"ply", "only"}
	want := 1.0 / 2.0
	if got := LyAdverbDensity(tokens); !floatEq(got, want) {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func TestLyAdverbDensity_NoAdverbs(t *testing.T) {
	tokens := []string{"run", "test", "build", "check"}
	if got := LyAdverbDensity(tokens); !floatEq(got, 0.0) {
		t.Fatalf("expected 0.0, got %v", got)
	}
}

func TestLyAdverbDensity_AllAdverbs(t *testing.T) {
	tokens := []string{"quickly", "slowly", "clearly", "simply"}
	if got := LyAdverbDensity(tokens); !floatEq(got, 1.0) {
		t.Fatalf("expected 1.0, got %v", got)
	}
}
