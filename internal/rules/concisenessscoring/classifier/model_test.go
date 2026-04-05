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
		"compression_ratio":   1,
		"type_token_ratio":    1,
		"nominal_density":     1,
		"sent_len_variance":   1,
		"func_word_ratio":     1,
		"avg_word_length":     1,
		"ly_adverb_density":   1,
		"unexpected":          1,
	})
	if err == nil {
		t.Fatal("expected unknown key error")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractFeatures_NewFeatures(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	text := "Basically, it seems that we are just trying to explain the same idea in order to make it very clear."
	result := model.Classify(text)

	// All 15 features should be present
	expectedFeatures := []string{
		"filler_rate", "hedge_rate", "verbose_phrase_rate",
		"modal_rate", "vague_rate", "action_rate",
		"content_ratio", "log_word_count",
		"compression_ratio", "type_token_ratio", "nominal_density",
		"sent_len_variance", "func_word_ratio", "avg_word_length",
		"ly_adverb_density",
	}
	for _, name := range expectedFeatures {
		if _, ok := result.FeatureSummary[name]; !ok {
			t.Errorf("missing feature %q in FeatureSummary", name)
		}
	}
	if len(result.FeatureSummary) != 15 {
		t.Errorf("expected 15 features, got %d", len(result.FeatureSummary))
	}
}

func normalizeText(text string) string {
	tokens := wordPattern.FindAllString(strings.ToLower(text), -1)
	if len(tokens) == 0 {
		return " "
	}
	return " " + strings.Join(tokens, " ") + " "
}

func TestCountPhraseMatches_UsesBoundaries(t *testing.T) {
	norm := normalizeText("This statement is in order too noisy to match the cue.")
	count, cues := countPhraseMatches(norm, []string{"in order to"})
	if count != 0 {
		t.Fatalf("expected 0 phrase matches, got %d (cues=%v)", count, cues)
	}

	norm = normalizeText("This statement is in order to reduce noise.")
	count, cues = countPhraseMatches(norm, []string{"in order to"})
	if count != 1 {
		t.Fatalf("expected 1 phrase match, got %d (cues=%v)", count, cues)
	}
}

func TestClassify_WordCount(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}
	result := model.Classify("hello world foo bar")
	if result.WordCount != 4 {
		t.Fatalf("expected WordCount=4, got %d", result.WordCount)
	}
}

func TestClassify_ActionWordsNotInCues(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}
	result := model.Classify("Run and build the test then deploy the update.")
	for _, cue := range result.TriggeredCues {
		for _, action := range []string{"run", "build", "test", "deploy", "update"} {
			if cue == action {
				t.Errorf("action word %q should not appear in TriggeredCues", cue)
			}
		}
	}
}

func TestClassify_EmptyInputKeepsCueSliceNonNil(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	result := model.Classify("")
	if result.TriggeredCues == nil {
		t.Fatal("expected non-nil TriggeredCues slice for empty input")
	}
	if len(result.TriggeredCues) != 0 {
		t.Fatalf(
			"expected zero triggered cues for empty input, got %d",
			len(result.TriggeredCues),
		)
	}
}

func BenchmarkClassify(b *testing.B) {
	model, err := LoadEmbedded()
	if err != nil {
		b.Fatalf("LoadEmbedded returned error: %v", err)
	}
	text := "Basically, it seems that we are just trying to explain " +
		"the same idea in order to make it very clear, and it " +
		"appears that we are really saying very little new " +
		"information overall."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model.Classify(text)
	}
}
