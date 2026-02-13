package paragraphreadability

import (
	"math"
	"testing"
)

func TestARI_Empty(t *testing.T) {
	got := ARI("")
	if got != 0 {
		t.Errorf("ARI of empty text: got %.2f, want 0", got)
	}
}

func TestARI_SingleWord(t *testing.T) {
	got := ARI("hello")
	// 1 word, 1 sentence, 5 chars
	// 4.71*(5/1) + 0.5*(1/1) - 21.43 = 23.55 + 0.5 - 21.43 = 2.62
	expected := 2.62
	if math.Abs(got-expected) > 0.5 {
		t.Errorf("ARI of single word: got %.2f, want ~%.2f", got, expected)
	}
}

func TestARI_SimpleText(t *testing.T) {
	// "The cat sat on the mat." -> 6 words, 1 sentence, 17 chars
	// 4.71*(17/6) + 0.5*(6/1) - 21.43
	// = 4.71*2.833 + 3.0 - 21.43
	// = 13.35 + 3.0 - 21.43 = -5.08
	got := ARI("The cat sat on the mat.")
	expected := -5.08
	if math.Abs(got-expected) > 0.5 {
		t.Errorf("ARI of simple text: got %.2f, want ~%.2f", got, expected)
	}
}

func TestARI_ComplexText(t *testing.T) {
	// Longer, more complex text should yield a higher readability grade.
	text := "The implementation of concurrent distributed systems " +
		"requires sophisticated understanding of fundamental " +
		"computational paradigms. Synchronization mechanisms " +
		"must guarantee linearizability across heterogeneous " +
		"processing environments."
	got := ARI(text)
	// This text has long words so ARI should be high (above 14).
	if got < 14 {
		t.Errorf(
			"ARI of complex text: got %.2f, expected > 14",
			got,
		)
	}
}

func TestARI_TwoSentences(t *testing.T) {
	text := "I am here. You are there."
	// 6 words, 2 sentences, 16 chars (Iamhereyouarethere = 18 chars)
	// Actually: I(1) a(1) m(1) h(1) e(1) r(1) e(1) Y(1) o(1) u(1)
	// a(1) r(1) e(1) t(1) h(1) e(1) r(1) e(1) = 18 chars
	// 4.71*(18/6) + 0.5*(6/2) - 21.43
	// = 4.71*3 + 1.5 - 21.43 = 14.13 + 1.5 - 21.43 = -5.8
	got := ARI(text)
	expected := -5.8
	if math.Abs(got-expected) > 0.5 {
		t.Errorf(
			"ARI of two sentences: got %.2f, want ~%.2f",
			got, expected,
		)
	}
}
