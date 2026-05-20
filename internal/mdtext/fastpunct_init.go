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
// `data.MustAsset` and the `LoadTraining` check below panic if the
// bundled English Punkt data is missing or malformed. Both are
// build-time invariants — the asset ships embedded in the binary
// — but a descriptive panic at init beats a nil dereference deep
// in tokenAnnotation when the asset is ever rebuilt incorrectly.
func buildTokenizer() *sentlib.DefaultSentenceTokenizer {
	raw := data.MustAsset("data/english.json")
	training, err := sentlib.LoadTraining(raw)
	if err != nil || training == nil {
		panic("mdtext: failed to load embedded english.json Punkt training data: " + errString(err))
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

// errString renders an error as a non-empty string for the panic
// message. A non-nil err yields its Error() text; a nil err (which
// can pair with a nil training value when LoadTraining returns
// (nil, nil)) renders as "training is nil".
func errString(err error) string {
	if err == nil {
		return "training is nil"
	}
	return err.Error()
}
