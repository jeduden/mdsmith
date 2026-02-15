package paragraphconciseness

import "testing"

func TestScore_TracksFillerHedgeAndVerboseHits(t *testing.T) {
	model := newScoringModel(
		defaultMinContentRate,
		[]string{"basically", "really"},
		[]string{"in most cases"},
		[]string{"in order to"},
	)

	score := model.score(
		"In order to basically explain this plan, we in most cases " +
			"really add extra words in order to make sure the idea is clear.",
	)

	if score.FillerHits < 2 {
		t.Errorf("expected filler hits >= 2, got %d", score.FillerHits)
	}
	if score.HedgeHits < 1 {
		t.Errorf("expected hedge hits >= 1, got %d", score.HedgeHits)
	}
	if score.VerboseHits < 2 {
		t.Errorf("expected verbose hits >= 2, got %d", score.VerboseHits)
	}
	if score.Verbosity <= 0 {
		t.Errorf("expected verbosity > 0, got %f", score.Verbosity)
	}
	if score.Conciseness >= 100 {
		t.Errorf("expected conciseness < 100, got %f", score.Conciseness)
	}
	if len(score.Samples) == 0 {
		t.Fatal("expected sample phrases, got none")
	}
}

func TestScore_ContentRatioPenalty(t *testing.T) {
	model := newScoringModel(0.8, nil, nil, nil)
	score := model.score(
		"It is the way that we are in the process of doing it in the way " +
			"that is in the doc.",
	)

	if score.ContentRatio >= 0.8 {
		t.Errorf(
			"expected content ratio < 0.8, got %f",
			score.ContentRatio,
		)
	}
	if score.Verbosity <= 0 {
		t.Errorf("expected verbosity > 0, got %f", score.Verbosity)
	}
}

func TestScore_EmptyText(t *testing.T) {
	model := newScoringModel(defaultMinContentRate, nil, nil, nil)
	score := model.score("")

	if score.TotalWords != 0 {
		t.Errorf("expected 0 words, got %d", score.TotalWords)
	}
	if score.Conciseness != 100 {
		t.Errorf("expected conciseness 100, got %f", score.Conciseness)
	}
}
