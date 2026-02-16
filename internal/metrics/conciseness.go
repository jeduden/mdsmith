package metrics

import (
	"math"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/mdtext"
)

var tokenPattern = regexp.MustCompile(`[a-z0-9']+`)

// Filler words and hedges reduce conciseness when overused.
var fillerWords = map[string]struct{}{
	"actually":  {},
	"basically": {},
	"clearly":   {},
	"generally": {},
	"just":      {},
	"kind":      {},
	"maybe":     {},
	"might":     {},
	"pretty":    {},
	"quite":     {},
	"really":    {},
	"simply":    {},
	"somewhat":  {},
	"very":      {},
}

// Stop words are excluded when estimating lexical density.
var stopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {},
	"be": {}, "but": {}, "by": {}, "for": {}, "from": {}, "if": {},
	"in": {}, "into": {}, "is": {}, "it": {}, "its": {}, "of": {},
	"on": {}, "or": {}, "that": {}, "the": {}, "their": {}, "then": {},
	"there": {}, "these": {}, "they": {}, "this": {}, "to": {}, "was": {},
	"we": {}, "were": {}, "will": {}, "with": {}, "you": {}, "your": {},
}

// Verbose phrases are penalized by distinct phrase presence.
var verbosePhrases = []string{
	"in order to",
	"make sure",
	"on the same page",
	"it is important to note",
	"in most cases",
}

func concisenessScore(text string) float64 {
	lower := strings.ToLower(text)
	tokens := tokenPattern.FindAllString(lower, -1)
	if len(tokens) == 0 {
		return 100.0
	}

	contentWords := 0
	fillerCount := 0
	for _, tok := range tokens {
		if _, ok := stopWords[tok]; !ok {
			contentWords++
		}
		if _, ok := fillerWords[tok]; ok {
			fillerCount++
		}
	}

	lexicalDensity := float64(contentWords) / float64(len(tokens))
	fillerRatio := float64(fillerCount) / float64(len(tokens))

	sentences := mdtext.CountSentences(text)
	if sentences < 1 {
		sentences = 1
	}

	avgSentenceWords := float64(len(tokens)) / float64(sentences)
	lengthPenalty := clamp((avgSentenceWords-24.0)/24.0, 0, 1)

	phraseHits := 0
	for _, phrase := range verbosePhrases {
		if strings.Contains(lower, phrase) {
			phraseHits++
		}
	}
	phrasePenalty := clamp(float64(phraseHits)/4.0, 0, 1)

	base := 100.0 * (0.65*lexicalDensity + 0.35*(1.0-fillerRatio))
	score := base - (22.0 * lengthPenalty) - (15.0 * phrasePenalty)
	score = clamp(score, 0, 100)

	return math.Round(score*10) / 10
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
