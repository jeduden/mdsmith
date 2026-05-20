//go:build !mdtext_punkt_upstream

package mdtext

import (
	sentlib "github.com/neurosnap/sentences"
	"github.com/neurosnap/sentences/data"
	"github.com/neurosnap/sentences/english"
)

// buildTokenizer assembles the same DefaultSentenceTokenizer that
// `english.NewSentenceTokenizer(nil)` would build — the same trained
// English data, the same word tokenizer, the same supervised
// abbreviations — but replaces the third-pass
// MultiPunctWordAnnotation with fastMultiPunctWordAnnotation. The
// only call-site difference is that the abbreviation classifier
// runs matchAbbrPattern in place of `reAbbr.FindAllString`. See
// `english/main.go:NewSentenceTokenizer` for the upstream original
// and plan 191 for the rationale.
//
// The upstream constructor swallows the data-load error
// (`t, _ := english.NewSentenceTokenizer(nil)` in the original
// initTokenizer), so this builder follows the same contract:
// embedded data is expected to succeed, and if it ever did not, the
// returned tokenizer would be `nil` and the first Tokenize call
// would panic — the same failure mode as before plan 191.
func buildTokenizer() *sentlib.DefaultSentenceTokenizer {
	raw, err := data.Asset("data/english.json")
	if err != nil {
		return nil
	}
	training, err := sentlib.LoadTraining(raw)
	if err != nil {
		return nil
	}
	// Supervised abbreviations applied by english.NewSentenceTokenizer.
	for _, abbr := range []string{"sgt", "gov", "no"} {
		training.AbbrevTypes.Add(abbr)
	}

	lang := sentlib.NewPunctStrings()
	word := english.NewWordTokenizer(lang)

	annotations := sentlib.NewAnnotations(training, lang, word)

	ortho := &sentlib.OrthoContext{
		Storage:      training,
		PunctStrings: lang,
		TokenType:    word,
		TokenFirst:   word,
	}

	fastMulti := &fastMultiPunctWordAnnotation{
		Storage:      training,
		TokenParser:  word,
		TokenGrouper: &sentlib.DefaultTokenGrouper{},
		Ortho:        ortho,
		upstreamWord: word,
	}
	annotations = append(annotations, fastMulti)

	return &sentlib.DefaultSentenceTokenizer{
		Storage:       training,
		PunctStrings:  lang,
		WordTokenizer: word,
		Annotations:   annotations,
	}
}
