package classifier

import (
	"math"
	"strings"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	if model.ModelID() == "" {
		t.Fatal("expected non-empty model id")
	}
	if model.Version() == "" {
		t.Fatal("expected non-empty version")
	}
	if model.Threshold() <= 0 || model.Threshold() >= 1 {
		t.Fatalf("expected threshold in (0,1), got %v", model.Threshold())
	}
	if EmbeddedArtifactBytes() <= 0 {
		t.Fatalf("expected embedded artifact bytes > 0")
	}
}

func TestClassifyDeterministic(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	text := "It seems this approach may potentially help in many cases."
	base := model.Classify(text)
	for i := 0; i < 20; i++ {
		got := model.Classify(text)
		if got.Label != base.Label {
			t.Fatalf("run %d label mismatch: got %q want %q", i, got.Label, base.Label)
		}
		if math.Abs(got.RiskScore-base.RiskScore) > 1e-12 {
			t.Fatalf(
				"run %d risk mismatch: got %.12f want %.12f",
				i,
				got.RiskScore,
				base.RiskScore,
			)
		}
	}
}

func TestClassifySeparatesVerboseAndDirect(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	verbose := model.Classify(
		"Basically, it seems we might want to consider this approach " +
			"in order to improve outcomes in many situations.",
	)
	direct := model.Classify(
		"Run go test ./... and publish checksums for release artifacts.",
	)

	if verbose.RiskScore <= direct.RiskScore {
		t.Fatalf(
			"expected verbose risk > direct risk, got %.4f <= %.4f",
			verbose.RiskScore,
			direct.RiskScore,
		)
	}
	if verbose.Label != "verbose-actionable" {
		t.Fatalf("expected verbose label, got %q", verbose.Label)
	}
	if direct.Label != "acceptable" {
		t.Fatalf("expected acceptable label, got %q", direct.Label)
	}
}

func TestLoadEmbedded_LexiconCoverage(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	counts := model.LexiconCounts()
	if counts.FillerWords < minFillerWords {
		t.Fatalf(
			"expected filler_words >= %d, got %d",
			minFillerWords,
			counts.FillerWords,
		)
	}
	if counts.ModalWords < minModalWords {
		t.Fatalf(
			"expected modal_words >= %d, got %d",
			minModalWords,
			counts.ModalWords,
		)
	}
	if counts.VagueWords < minVagueWords {
		t.Fatalf(
			"expected vague_words >= %d, got %d",
			minVagueWords,
			counts.VagueWords,
		)
	}
	if counts.ActionWords < minActionWords {
		t.Fatalf(
			"expected action_words >= %d, got %d",
			minActionWords,
			counts.ActionWords,
		)
	}
	if counts.StopWords < minStopWords {
		t.Fatalf(
			"expected stop_words >= %d, got %d",
			minStopWords,
			counts.StopWords,
		)
	}
	if counts.HedgePhrases < minHedgePhrases {
		t.Fatalf(
			"expected hedge_phrases >= %d, got %d",
			minHedgePhrases,
			counts.HedgePhrases,
		)
	}
	if counts.VerbosePhrases < minVerbosePhrases {
		t.Fatalf(
			"expected verbose_phrases >= %d, got %d",
			minVerbosePhrases,
			counts.VerbosePhrases,
		)
	}
}

func TestNormalizeCueList_RejectsDuplicatesAfterNormalization(t *testing.T) {
	_, err := normalizeCueList("filler_words", []string{"Maybe", "maybe"}, 1, true)
	if err == nil {
		t.Fatal("expected duplicate normalization error")
	}
	if !strings.Contains(err.Error(), "duplicate entry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeCueList_RejectsInvalidToken(t *testing.T) {
	_, err := normalizeCueList("filler_words", []string{"two words"}, 1, true)
	if err == nil {
		t.Fatal("expected invalid token error")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWeights_RejectsUnknownKey(t *testing.T) {
	err := validateWeights(map[string]float64{
		"filler_rate":         1,
		"hedge_rate":          1,
		"verbose_phrase_rate": 1,
		"modal_rate":          1,
		"vague_rate":          1,
		"action_rate":         1,
		"content_ratio":       1,
		"log_word_count":      1,
		"unexpected":          1,
	})
	if err == nil {
		t.Fatal("expected unknown key error")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("unexpected error: %v", err)
	}
}
