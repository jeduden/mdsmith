//go:build mdtext_punkt_upstream

package mdtext

import (
	"strings"

	sentlib "github.com/neurosnap/sentences"
	"github.com/neurosnap/sentences/english"
)

// upstreamTok is the upstream-build singleton — the unmodified
// english.NewSentenceTokenizer(nil) pipeline. This file is selected
// only when -tags mdtext_punkt_upstream is set, which exists so a
// developer can A/B-compare segmentation under the upstream code path
// without touching the fork. The equivalence harness in
// sentence_equivalence_test.go runs under both builds.
//
// The upstream constructor's data-load error is swallowed to
// preserve the original `t, _ := english.NewSentenceTokenizer(nil)`
// shape this file is meant to verify against.
var upstreamTok *sentlib.DefaultSentenceTokenizer

func initTokenizer() {
	upstreamTok, _ = english.NewSentenceTokenizer(nil)
}

// splitSentencesInto is the upstream-build segmentation
// implementation. Same trim-and-filter behaviour and dst-pooling
// contract as the default build's splitSentencesInto so the
// SplitSentences and SplitSentencesInto entry points produce
// identical output across both tags.
func splitSentencesInto(dst []string, text string) []string {
	sents := upstreamTok.Tokenize(text)
	if dst == nil {
		dst = make([]string, 0, len(sents))
	}
	for _, s := range sents {
		t := strings.TrimSpace(s.Text)
		if t != "" {
			dst = append(dst, t)
		}
	}
	return dst
}
