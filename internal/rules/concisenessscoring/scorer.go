package concisenessscoring

import (
	"github.com/jeduden/mdsmith/internal/rules/concisenessscoring/classifier"
)

// Scorer wraps the classifier model to produce conciseness scores.
type Scorer struct {
	model *classifier.Model
}

// ScoreResult holds the conciseness score for a paragraph.
type ScoreResult struct {
	// Conciseness is a float64 in [0, 1] where 1.0 means maximally concise.
	Conciseness float64
	// WordCount is the number of words in the paragraph.
	WordCount int
	// Cues lists the triggered verbose cues for diagnostic messages.
	Cues []string
}

// NewScorer loads the embedded classifier and returns a Scorer.
func NewScorer() (*Scorer, error) {
	m, err := classifier.LoadEmbedded()
	if err != nil {
		return nil, err
	}
	return &Scorer{model: m}, nil
}

// Score computes the conciseness score for a paragraph.
// The classifier produces a RiskScore where high = verbose.
// Conciseness is 1 - RiskScore, so high = concise.
func (s *Scorer) Score(text string) ScoreResult {
	r := s.model.Classify(text)
	return ScoreResult{
		Conciseness: 1.0 - r.RiskScore,
		WordCount:   r.WordCount,
		Cues:        r.TriggeredCues,
	}
}
