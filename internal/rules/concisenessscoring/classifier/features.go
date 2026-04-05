package classifier

import (
	"math"
	"regexp"
	"strings"
)

var sentPattern = regexp.MustCompile(`[.!?]+`)

// funcWords is the hardcoded set of determiners, prepositions, conjunctions,
// and pronouns used by FuncWordRatio.
var funcWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "and": {}, "but": {}, "or": {}, "nor": {},
	"for": {}, "yet": {}, "so": {}, "in": {}, "on": {}, "at": {}, "to": {},
	"of": {}, "by": {}, "from": {}, "with": {}, "as": {}, "into": {},
	"through": {}, "during": {}, "before": {}, "after": {}, "above": {},
	"below": {}, "between": {}, "under": {}, "about": {}, "against": {},
	"he": {}, "she": {}, "it": {}, "they": {}, "we": {}, "i": {}, "you": {},
	"me": {}, "him": {}, "her": {}, "us": {}, "them": {},
	"my": {}, "your": {}, "his": {}, "its": {}, "our": {}, "their": {},
	"this": {}, "that": {}, "these": {}, "those": {},
}

// nominalizationSuffixes is the list of suffixes used by NominalDensity.
var nominalizationSuffixes = []string{
	"tion", "ment", "ness", "ity", "ance", "ence",
}

// CompressionRatio estimates text redundancy using bigram repetition.
// It returns the fraction of repeated token bigrams. Higher values
// indicate more repetitive (redundant) text. Returns 0.0 if
// tokens has fewer than 2 entries.
func CompressionRatio(tokens []string) float64 {
	if len(tokens) < 2 {
		return 0.0
	}
	total := len(tokens) - 1
	seen := make(map[string]struct{}, total)
	repeated := 0
	for i := 0; i < total; i++ {
		bigram := tokens[i] + " " + tokens[i+1]
		if _, ok := seen[bigram]; ok {
			repeated++
		} else {
			seen[bigram] = struct{}{}
		}
	}
	return float64(repeated) / float64(total)
}

// TypeTokenRatio returns the ratio of unique tokens to total tokens.
// Higher values indicate more varied vocabulary. Returns 0.0 for an empty
// slice.
func TypeTokenRatio(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0.0
	}
	seen := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		seen[t] = struct{}{}
	}
	return float64(len(seen)) / float64(len(tokens))
}

// NominalDensity returns the fraction of tokens ending in common
// nominalization suffixes (-tion, -ment, -ness, -ity, -ance, -ence).
// Returns 0.0 for an empty slice.
func NominalDensity(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0.0
	}
	count := 0
	for _, t := range tokens {
		for _, suf := range nominalizationSuffixes {
			if strings.HasSuffix(t, suf) {
				count++
				break
			}
		}
	}
	return float64(count) / float64(len(tokens))
}

// SentLenVariance splits text into sentences on `.`, `!`, `?` and returns
// the coefficient of variation (stddev / mean) of sentence word counts.
// Returns 0.0 when fewer than 2 sentences are found.
func SentLenVariance(text string) float64 {
	parts := sentPattern.Split(text, -1)
	// Collect non-empty sentences.
	var lengths []float64
	for _, p := range parts {
		words := wordPattern.FindAllString(strings.ToLower(p), -1)
		if len(words) > 0 {
			lengths = append(lengths, float64(len(words)))
		}
	}
	if len(lengths) < 2 {
		return 0.0
	}
	var sum float64
	for _, l := range lengths {
		sum += l
	}
	mean := sum / float64(len(lengths))
	if mean == 0 {
		return 0.0
	}
	var variance float64
	for _, l := range lengths {
		d := l - mean
		variance += d * d
	}
	variance /= float64(len(lengths))
	return math.Sqrt(variance) / mean
}

// FuncWordRatio returns the fraction of tokens that are function words
// (determiners, prepositions, conjunctions, pronouns). Returns 0.0 for an
// empty slice.
func FuncWordRatio(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0.0
	}
	count := 0
	for _, t := range tokens {
		if _, ok := funcWords[t]; ok {
			count++
		}
	}
	return float64(count) / float64(len(tokens))
}

// AvgWordLength returns the mean character length of tokens. Returns 0.0 for
// an empty slice.
func AvgWordLength(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0.0
	}
	total := 0
	for _, t := range tokens {
		total += len(t)
	}
	return float64(total) / float64(len(tokens))
}

// LyAdverbDensity returns the fraction of tokens ending in "ly" with length
// >= 4 (to exclude short words like "fly"). Returns 0.0 for an empty slice.
func LyAdverbDensity(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0.0
	}
	count := 0
	for _, t := range tokens {
		if len(t) >= 4 && strings.HasSuffix(t, "ly") {
			count++
		}
	}
	return float64(count) / float64(len(tokens))
}
