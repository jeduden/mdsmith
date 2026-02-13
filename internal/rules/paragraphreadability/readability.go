package paragraphreadability

import "github.com/jeduden/tidymark/internal/mdtext"

// GradeFunc computes a readability grade level from plain text.
// Higher values mean harder to read.
type GradeFunc func(text string) float64

// ARI computes the Automated Readability Index.
// Formula: 4.71*(characters/words) + 0.5*(words/sentences) - 21.43
// Characters = letters and digits only.
func ARI(text string) float64 {
	words := mdtext.CountWords(text)
	if words == 0 {
		return 0
	}
	sentences := mdtext.CountSentences(text)
	characters := mdtext.CountCharacters(text)

	return 4.71*float64(characters)/float64(words) +
		0.5*float64(words)/float64(sentences) -
		21.43
}
