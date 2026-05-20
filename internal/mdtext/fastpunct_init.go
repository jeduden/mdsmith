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
func buildTokenizer() *sentlib.DefaultSentenceTokenizer {
	training := mustLoadTraining(data.MustAsset("data/english.json"))

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

// mustLoadTraining parses raw Punkt training JSON and panics with a
// descriptive message on malformed input or nil result. Extracted
// from buildTokenizer so the failure branch can be driven red/green
// with malformed bytes — see TestMustLoadTraining_PanicsOn*.
func mustLoadTraining(raw []byte) *sentlib.Storage {
	training, err := sentlib.LoadTraining(raw)
	if err != nil {
		panic("mdtext: failed to load Punkt training data: " + err.Error())
	}
	if training == nil {
		panic("mdtext: Punkt training data is nil")
	}
	return training
}
