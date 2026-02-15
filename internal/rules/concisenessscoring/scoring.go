package concisenessscoring

import (
	"math"
	"regexp"
	"strings"
)

const (
	fillerWeight  = 1.0
	hedgeWeight   = 1.2
	verboseWeight = 1.4
)

var (
	wordPattern = regexp.MustCompile(`[a-z0-9']+`)
	stopWords   = map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {},
		"be": {}, "by": {}, "for": {}, "from": {}, "has": {}, "have": {},
		"in": {}, "is": {}, "it": {}, "its": {}, "of": {}, "on": {},
		"or": {}, "that": {}, "the": {}, "their": {}, "there": {},
		"these": {}, "this": {}, "to": {}, "was": {}, "were": {},
		"with": {},
	}
)

type heuristics struct {
	fillerSet      map[string]struct{}
	hedgePhrases   []string
	verbosePhrases []string
}

type scoreResult struct {
	Score     float64
	WordCount int
	Examples  []string
}

func newHeuristics(
	fillerWords []string, hedgePhrases []string, verbosePhrases []string,
) heuristics {
	return heuristics{
		fillerSet:      toWordSet(fillerWords),
		hedgePhrases:   normalizePhrases(hedgePhrases),
		verbosePhrases: normalizePhrases(verbosePhrases),
	}
}

func scoreParagraph(text string, heur heuristics) scoreResult {
	tokens := tokenizeWords(text)
	if len(tokens) == 0 {
		return scoreResult{Score: 1.0}
	}

	joined := " " + strings.Join(tokens, " ") + " "
	total := len(tokens)

	contentWords := 0
	fillerHits := 0
	examples := make([]string, 0, 2)
	seenExamples := make(map[string]struct{})

	for _, token := range tokens {
		if _, ok := heur.fillerSet[token]; ok {
			fillerHits++
			addExample(&examples, seenExamples, token)
		}
		if len(token) >= 4 {
			if _, isStopWord := stopWords[token]; !isStopWord {
				contentWords++
			}
		}
	}

	hedgeHits := countPhraseHits(
		joined, heur.hedgePhrases, &examples, seenExamples,
	)
	verboseHits := countPhraseHits(
		joined, heur.verbosePhrases, &examples, seenExamples,
	)

	contentRatio := float64(contentWords) / float64(total)
	fillerRatio := float64(fillerHits) / float64(total)
	hedgeRatio := float64(hedgeHits) / float64(total)
	verboseRatio := float64(verboseHits) / float64(total)

	score := contentRatio -
		(fillerWeight * fillerRatio) -
		(hedgeWeight * hedgeRatio) -
		(verboseWeight * verboseRatio)
	score = math.Max(0, math.Min(score, 1))

	return scoreResult{
		Score:     score,
		WordCount: total,
		Examples:  examples,
	}
}

func tokenizeWords(text string) []string {
	return wordPattern.FindAllString(strings.ToLower(text), -1)
}

func toWordSet(words []string) map[string]struct{} {
	out := make(map[string]struct{}, len(words))
	for _, word := range words {
		tokens := tokenizeWords(word)
		if len(tokens) == 0 {
			continue
		}
		out[tokens[0]] = struct{}{}
	}
	return out
}

func normalizePhrases(phrases []string) []string {
	out := make([]string, 0, len(phrases))
	for _, phrase := range phrases {
		tokens := tokenizeWords(phrase)
		if len(tokens) == 0 {
			continue
		}
		out = append(out, strings.Join(tokens, " "))
	}
	return out
}

func countPhraseHits(
	text string, phrases []string, examples *[]string, seen map[string]struct{},
) int {
	hits := 0
	for _, phrase := range phrases {
		marker := " " + phrase + " "
		n := strings.Count(text, marker)
		if n == 0 {
			continue
		}
		hits += n
		addExample(examples, seen, phrase)
	}
	return hits
}

func addExample(
	examples *[]string, seen map[string]struct{}, value string,
) {
	if len(*examples) >= 3 {
		return
	}
	if _, ok := seen[value]; ok {
		return
	}
	seen[value] = struct{}{}
	*examples = append(*examples, value)
}
