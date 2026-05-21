package punkt

import (
	"sync"

	"github.com/neurosnap/sentences/data"
)

// Sentence carries the [start, end) byte slice of a sentence inside
// the original text. The Text field is computed by Tokenize so the
// caller need not slice the source itself; the slice header points
// into the original string, so no copy.
type Sentence struct {
	Start int
	End   int
	Text  string
}

// Tokenizer is the public entry point of the fork. Goroutine-safe:
// the per-call state lives in a sync.Pool, so concurrent Tokenize
// calls each get an independent buffer set.
type Tokenizer struct {
	Storage *Storage
	ortho   *OrthoContext
	pool    sync.Pool
}

// NewEnglish constructs the fork's equivalent of upstream
// english.NewSentenceTokenizer(nil): loads the bundled English
// training data, applies the same three supervised abbreviations
// (sgt, gov, no), and assembles the three annotators TypeBased,
// TokenBased, MultiPunct.
func NewEnglish() *Tokenizer {
	return New(loadEnglishStorage(data.MustAsset("data/english.json")))
}

// loadEnglishStorage parses raw English Punkt training data into a
// Storage and applies the three supervised abbreviations upstream
// adds (sgt, gov, no). Extracted from NewEnglish so the failure
// branch can be driven red/green with malformed bytes — see
// TestLoadEnglishStorage_PanicsOnMalformed.
func loadEnglishStorage(raw []byte) *Storage {
	storage, err := LoadTraining(raw)
	if err != nil {
		panic("punkt: failed to load English training data: " + err.Error())
	}
	for _, abbr := range []string{"sgt", "gov", "no"} {
		storage.AbbrevTypes.Add(abbr)
	}
	return storage
}

// New constructs a Tokenizer over an arbitrary Storage. Used by
// tests that need a hermetic, synthetic Storage; production code
// goes through NewEnglish.
func New(s *Storage) *Tokenizer {
	t := &Tokenizer{Storage: s}
	t.ortho = &OrthoContext{Storage: s}
	t.pool.New = func() any {
		return &state{
			tokens:    make([]Token, 0, 64),
			ptrs:      make([]*Token, 0, 64),
			pairs:     make([][2]*Token, 0, 64),
			typeBufA:  make([]byte, 0, 32),
			typeBufB:  make([]byte, 0, 32),
			colKeyBuf: make([]byte, 0, 64),
			sents:     make([]Sentence, 0, 16),
		}
	}
	return t
}

// Tokenize splits text into Sentences. Mirrors upstream
// DefaultSentenceTokenizer.Tokenize but reuses pooled state so the
// per-call allocation count drops to a handful (the result slice
// itself, plus any buffer growth on a particularly long input).
//
// The Text field of each Sentence points into text via a substring
// (no copy). Callers that retain Sentences past the lifetime of text
// must copy Text first.
func (t *Tokenizer) Tokenize(text string) []Sentence {
	if text == "" {
		return nil
	}
	st := t.pool.Get().(*state)
	defer func() {
		st.reset()
		t.pool.Put(st)
	}()

	// 1. Tokenize text into st.tokens, then build st.ptrs. Tokenize
	// already short-circuited the empty-text case above, and
	// TokenizeInto's fallback emits at least one token for any
	// non-empty input, so st.tokens is guaranteed non-empty here.
	st.tokens = TokenizeInto(st.tokens, text, false)
	if cap(st.ptrs) < len(st.tokens) {
		st.ptrs = make([]*Token, 0, len(st.tokens))
	}
	st.ptrs = st.ptrs[:0]
	for i := range st.tokens {
		st.ptrs = append(st.ptrs, &st.tokens[i])
	}

	// 2. Run the three annotation passes.
	for _, pair := range group(st, st.ptrs) {
		typeAnnotation(t.Storage, pair[0])
	}
	for _, pair := range group(st, st.ptrs) {
		if pair[1] == nil {
			continue
		}
		tokenAnnotation(t.Storage, t.ortho, pair[0], pair[1], st)
	}
	for _, pair := range group(st, st.ptrs) {
		if pair[1] == nil {
			continue
		}
		multiPunctAnnotation(t.Storage, t.ortho, pair[0], pair[1], st)
	}

	// 3. Walk annotated tokens and emit Sentences. Mirrors upstream
	// DefaultSentenceTokenizer.Tokenize.
	lastBreak := 0
	for i := range st.tokens {
		if !st.tokens[i].SentBreak {
			continue
		}
		st.sents = append(st.sents, Sentence{
			Start: lastBreak,
			End:   st.tokens[i].Position,
			Text:  text[lastBreak:st.tokens[i].Position],
		})
		lastBreak = st.tokens[i].Position
	}
	if lastBreak != len(text) {
		st.sents = append(st.sents, Sentence{
			Start: lastBreak,
			End:   len(text),
			Text:  text[lastBreak:],
		})
	}

	// 4. Copy out — the pooled state is reused on the next call, so
	// the returned slice must not alias st.sents.
	out := make([]Sentence, len(st.sents))
	copy(out, st.sents)
	return out
}
