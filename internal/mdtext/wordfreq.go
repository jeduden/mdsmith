package mdtext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// WordFrequency returns a case-folded word → count map for text.
// A word is a maximal run of Unicode letter or digit runes. Words
// shorter than minLength runes (by rune count, not byte count) are
// excluded. Pass minLength <= 0 to include all non-empty words.
//
// The caller is expected to pass plain text produced by
// [ExtractPlainText] so that fenced and indented code block content
// is already absent.
func WordFrequency(text string, minLength int) map[string]int {
	freq := make(map[string]int)
	WordFrequencyInto(freq, text, minLength)
	return freq
}

// WordFrequencyInto accumulates case-folded word counts from text into
// freq. It is the zero-allocation inner loop used by rules that maintain
// their own freq map across multiple paragraphs — pass in a map created
// once per scope unit and cleared (via the built-in clear) between units.
//
// Tokenisation rules are identical to [WordFrequency]: maximal runs of
// Unicode letter or digit runes, case-folded, words shorter than
// minLength runes excluded.
func WordFrequencyInto(freq map[string]int, text string, minLength int) {
	start := -1
	runes := 0
	for i := 0; i <= len(text); {
		var r rune
		var size int
		if i < len(text) {
			r, size = utf8.DecodeRuneInString(text[i:])
		} else {
			r = 0
			size = 1
		}
		if i < len(text) && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			if start < 0 {
				start = i
				runes = 0
			}
			runes++
		} else {
			if start >= 0 && (minLength <= 0 || runes >= minLength) {
				// strings.ToLower returns the original string unchanged
				// when no uppercase rune is present, so typical all-lowercase
				// prose words cost zero allocation here.
				freq[strings.ToLower(text[start:i])]++
			}
			start = -1
		}
		i += size
	}
}

// MaxWordFrequency returns the highest count in freq, or 0 if freq is
// empty or nil.
func MaxWordFrequency(freq map[string]int) int {
	max := 0
	for _, n := range freq {
		if n > max {
			max = n
		}
	}
	return max
}
