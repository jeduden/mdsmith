package mdtext

import (
	"sync"

	"github.com/jeduden/mdsmith/internal/punkt"
)

// abbrevStorage lazily loads the trained English Punkt model for
// abbreviation lookups. It carries no //go:build tag and is shared by
// both build tags: fastpunct_init.go's forkTokenizer is constructed
// from this same Storage instead of parsing a second copy, and the
// mdtext_punkt_upstream build (whose neurosnap/sentences tokenizer has
// no abbreviation API of its own) reads it directly. Abbreviation
// lookup has exactly one implementation regardless of which
// sentence-tokenizer backend SplitSentences uses, so it does not need
// a per-build-tag copy the way splitSentencesInto does.
var abbrevStorage = sync.OnceValue(func() *punkt.Storage {
	return punkt.NewEnglish().Storage
})

// AbbrevTrimCutset is the trailing clause punctuation IsAbbrevToken
// ignores internally, re-exported so a caller normalizing its own
// abbreviation list (e.g. line-length reflow's configured
// Abbreviations) can match the same trim instead of keeping a second
// copy of the cutset.
const AbbrevTrimCutset = punkt.AbbrevTokenCutset

// IsAbbrevToken reports whether tok is a known abbreviation per the
// trained Punkt model (honorifics like "Dr.", reference forms like
// "vs.", initials like "J.", and dotted forms like "e.g."). Callers
// that need abbreviation-aware behavior, such as line-length reflow,
// use this instead of loading their own internal/punkt model — only
// mdtext is allowed to import internal/punkt directly.
func IsAbbrevToken(tok string) bool {
	return abbrevStorage().IsAbbrevToken(tok)
}
