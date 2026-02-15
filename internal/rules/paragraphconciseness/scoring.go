package paragraphconciseness

import (
	"regexp"
	"sort"
	"strings"
)

const (
	fillerWeight  = 120.0
	hedgeWeight   = 140.0
	verboseWeight = 180.0
	contentWeight = 35.0
)

var (
	tokenPattern = regexp.MustCompile(`[\pL\pN]+`)

	defaultFillerWords = []string{
		"actually",
		"basically",
		"quite",
		"really",
		"simply",
		"very",
	}

	defaultHedgePhrases = []string{
		"in general",
		"in most cases",
		"it appears",
		"it seems",
		"kind of",
		"maybe",
		"perhaps",
		"probably",
		"sort of",
		"to some extent",
	}

	defaultVerbosePhrases = []string{
		"at this point in time",
		"be able to",
		"due to the fact that",
		"for the purpose of",
		"in order to",
		"in terms of",
		"in the event that",
		"it is important to note that",
		"make sure that",
		"the fact that",
	}

	defaultStopWords = map[string]struct{}{
		"a": {}, "about": {}, "above": {}, "after": {}, "again": {}, "against": {},
		"all": {}, "also": {}, "am": {}, "an": {}, "and": {}, "any": {}, "are": {},
		"as": {}, "at": {}, "be": {}, "because": {}, "been": {}, "before": {}, "being": {},
		"below": {}, "between": {}, "both": {}, "but": {}, "by": {}, "can": {},
		"could": {}, "did": {}, "do": {}, "does": {}, "doing": {}, "down": {}, "during": {},
		"each": {}, "few": {}, "for": {}, "from": {}, "further": {}, "had": {},
		"has": {}, "have": {}, "having": {}, "he": {}, "her": {}, "here": {}, "hers": {},
		"herself": {}, "him": {}, "himself": {}, "his": {}, "how": {}, "i": {}, "if": {},
		"in": {}, "into": {}, "is": {}, "it": {}, "its": {}, "itself": {}, "just": {},
		"me": {}, "more": {}, "most": {}, "my": {}, "myself": {}, "no": {}, "nor": {},
		"not": {}, "of": {}, "off": {}, "on": {}, "once": {}, "only": {}, "or": {},
		"other": {}, "our": {}, "ours": {}, "ourselves": {}, "out": {}, "over": {}, "own": {},
		"same": {}, "she": {}, "should": {}, "so": {}, "some": {}, "such": {}, "than": {},
		"that": {}, "the": {}, "their": {}, "theirs": {}, "them": {}, "themselves": {},
		"then": {}, "there": {}, "these": {}, "they": {}, "this": {}, "those": {}, "through": {},
		"to": {}, "too": {}, "under": {}, "until": {}, "up": {}, "very": {}, "was": {},
		"we": {}, "were": {}, "what": {}, "when": {}, "where": {}, "which": {}, "while": {},
		"who": {}, "whom": {}, "why": {}, "will": {}, "with": {}, "would": {}, "you": {},
		"your": {}, "yours": {}, "yourself": {}, "yourselves": {},
	}
)

type scoreBreakdown struct {
	TotalWords   int
	FillerHits   int
	HedgeHits    int
	VerboseHits  int
	ContentRatio float64
	Verbosity    float64
	Conciseness  float64
	Samples      []string
}

type scoringModel struct {
	minContentRatio float64
	fillerWords     map[string]struct{}
	hedgeSingles    map[string]struct{}
	hedgePhrases    [][]string
	verbosePhrases  [][]string
}

func newScoringModel(
	minContentRatio float64,
	fillerWords []string,
	hedgePhrases []string,
	verbosePhrases []string,
) scoringModel {
	fillers := normalizeWordList(fillerWords)
	hedges := compilePhrases(hedgePhrases)
	verbose := compilePhrases(verbosePhrases)
	fillerSet := make(map[string]struct{}, len(fillers))
	for _, w := range fillers {
		fillerSet[w] = struct{}{}
	}
	hedgeSingles := make(map[string]struct{})
	for _, phrase := range hedges {
		if len(phrase) == 1 {
			hedgeSingles[phrase[0]] = struct{}{}
		}
	}
	return scoringModel{
		minContentRatio: minContentRatio,
		fillerWords:     fillerSet,
		hedgeSingles:    hedgeSingles,
		hedgePhrases:    hedges,
		verbosePhrases:  verbose,
	}
}

func (m scoringModel) score(text string) scoreBreakdown {
	tokens := tokenizeWords(text)
	total := len(tokens)
	if total == 0 {
		return scoreBreakdown{
			Conciseness:  100,
			ContentRatio: 1,
		}
	}

	fillerHits, fillerSamples := countWordHits(tokens, m.fillerWords)
	hedgeHits, hedgeSamples := countPhraseHits(tokens, m.hedgePhrases)
	verboseHits, verboseSamples := countPhraseHits(tokens, m.verbosePhrases)
	contentRatio := m.contentRatio(tokens)

	fillerRatio := float64(fillerHits) / float64(total)
	hedgeRatio := float64(hedgeHits) / float64(total)
	verboseRatio := float64(verboseHits) / float64(total)
	verbosity := fillerRatio*fillerWeight +
		hedgeRatio*hedgeWeight +
		verboseRatio*verboseWeight
	if m.minContentRatio > 0 && contentRatio < m.minContentRatio {
		verbosity += ((m.minContentRatio - contentRatio) / m.minContentRatio) *
			contentWeight
	}
	verbosity = clamp(verbosity, 0, 100)
	conciseness := clamp(100-verbosity, 0, 100)

	samples := firstN(
		mergeUnique(verboseSamples, hedgeSamples, fillerSamples), 3,
	)
	return scoreBreakdown{
		TotalWords:   total,
		FillerHits:   fillerHits,
		HedgeHits:    hedgeHits,
		VerboseHits:  verboseHits,
		ContentRatio: contentRatio,
		Verbosity:    verbosity,
		Conciseness:  conciseness,
		Samples:      samples,
	}
}

func (m scoringModel) contentRatio(tokens []string) float64 {
	if len(tokens) == 0 {
		return 1
	}
	content := 0
	for _, tok := range tokens {
		if len(tok) <= 2 {
			continue
		}
		if _, ok := defaultStopWords[tok]; ok {
			continue
		}
		if _, ok := m.fillerWords[tok]; ok {
			continue
		}
		if _, ok := m.hedgeSingles[tok]; ok {
			continue
		}
		content++
	}
	return float64(content) / float64(len(tokens))
}

func countWordHits(
	tokens []string, words map[string]struct{},
) (int, []string) {
	if len(words) == 0 || len(tokens) == 0 {
		return 0, nil
	}
	hits := 0
	var samples []string
	seen := make(map[string]struct{})
	for _, tok := range tokens {
		if _, ok := words[tok]; !ok {
			continue
		}
		hits++
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		samples = append(samples, tok)
	}
	return hits, samples
}

func countPhraseHits(
	tokens []string, phrases [][]string,
) (int, []string) {
	if len(phrases) == 0 || len(tokens) == 0 {
		return 0, nil
	}
	hits := 0
	var samples []string
	seen := make(map[string]struct{})
	for i := 0; i < len(tokens); i++ {
		for _, phrase := range phrases {
			if i+len(phrase) > len(tokens) {
				continue
			}
			if !matchPhrase(tokens[i:i+len(phrase)], phrase) {
				continue
			}
			hits++
			text := strings.Join(phrase, " ")
			if _, ok := seen[text]; !ok {
				seen[text] = struct{}{}
				samples = append(samples, text)
			}
			break
		}
	}
	return hits, samples
}

func matchPhrase(tokens []string, phrase []string) bool {
	if len(tokens) != len(phrase) {
		return false
	}
	for i := range tokens {
		if tokens[i] != phrase[i] {
			return false
		}
	}
	return true
}

func tokenizeWords(text string) []string {
	return tokenPattern.FindAllString(strings.ToLower(text), -1)
}

func compilePhrases(phrases []string) [][]string {
	normalized := normalizePhraseList(phrases)
	out := make([][]string, 0, len(normalized))
	for _, p := range normalized {
		tokens := tokenizeWords(p)
		if len(tokens) == 0 {
			continue
		}
		out = append(out, tokens)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i]) > len(out[j])
	})
	return out
}

func normalizeWordList(words []string) []string {
	seen := make(map[string]struct{}, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.TrimSpace(strings.ToLower(w))
		if w == "" {
			continue
		}
		if len(tokenizeWords(w)) != 1 {
			continue
		}
		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		out = append(out, w)
	}
	return out
}

func normalizePhraseList(phrases []string) []string {
	seen := make(map[string]struct{}, len(phrases))
	out := make([]string, 0, len(phrases))
	for _, phrase := range phrases {
		tokens := tokenizeWords(phrase)
		if len(tokens) == 0 {
			continue
		}
		normalized := strings.Join(tokens, " ")
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func mergeUnique(groups ...[]string) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, item := range group {
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func firstN(items []string, n int) []string {
	if n < 0 || len(items) <= n {
		return items
	}
	return items[:n]
}

func clamp(v float64, min float64, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
