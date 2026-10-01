package classifier

import (
	"math"
	"strings"
	"unicode"
)

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

type bigramKey struct{ a, b int }

// CompressionRatio estimates text redundancy using bigram repetition.
// It returns the fraction of repeated token bigrams. Higher values
// indicate more repetitive (redundant) text. Returns 0.0 if
// tokens has fewer than 2 entries.
func CompressionRatio(tokens []string) float64 {
	if len(tokens) < 2 {
		return 0.0
	}
	intern := make(map[string]int, len(tokens))
	id := func(s string) int {
		if v, ok := intern[s]; ok {
			return v
		}
		v := len(intern)
		intern[s] = v
		return v
	}
	total := len(tokens) - 1
	seen := make(map[bigramKey]struct{}, total)
	repeated := 0
	for i := 0; i < total; i++ {
		key := bigramKey{id(tokens[i]), id(tokens[i+1])}
		if _, ok := seen[key]; ok {
			repeated++
		} else {
			seen[key] = struct{}{}
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
	// One pass, no allocation: count words per sentence while scanning
	// (high-performance-go.md, "Allocations"). A word is a run of
	// [a-z0-9'] after rune-wise lowercasing — the same tokens as
	// wordPattern in model.go (keep the two in step; the oracle test in
	// sentlen_test.go pins it) — and a sentence ends at each
	// run of '.', '!', '?'.
	// Welford's running mean/M2 over per-sentence word counts: exactly 0
	// for equal lengths, and no integer overflow on huge inputs.
	var n, words int
	var mean, m2 float64
	flush := func() {
		if words > 0 {
			n++
			w := float64(words)
			delta := w - mean
			mean += delta / float64(n)
			m2 += delta * (w - mean)
			words = 0
		}
	}
	inWord := false
	for _, r := range text {
		switch r {
		case '.', '!', '?':
			flush()
			inWord = false
			continue
		}
		if r >= 'A' && r <= 'Z' || r >= 0x80 {
			r = unicode.ToLower(r)
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '\'' {
			if !inWord {
				words++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	flush()
	if n < 2 {
		return 0.0
	}
	return math.Sqrt(m2/float64(n)) / mean
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
