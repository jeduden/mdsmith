//go:build !mdtext_punkt_upstream

package mdtext

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/punkt"
)

// forkTokenizer is the default-build singleton — a pooled,
// allocation-clean reimplementation of trained Punkt vendored in
// internal/punkt/. Plan 193 owns the rationale: the upstream pipeline
// allocates ~135 times per Tokenize call (six per-token regex
// pointers, per-pass grouper buffer, strings.Join for collocations,
// etc.); the fork's pooled state amortizes the whole thing to a
// handful of allocations per call.
//
// Construction happens once via initTokenizer (in mdtext.go) under
// initOnce. Subsequent SplitSentences calls reuse this singleton; the
// per-call state buffers live in a sync.Pool inside punkt.Tokenizer
// so concurrent callers do not contend.
var forkTokenizer *punkt.Tokenizer

// initTokenizer assembles the default-build tokenizer. The upstream
// build tag (mdtext_punkt_upstream) provides its own initTokenizer in
// upstreampunct.go.
func initTokenizer() {
	forkTokenizer = punkt.NewEnglish()
}

// splitSentencesInto is the default-build segmentation
// implementation. It runs the fork tokenizer, trims each
// Sentence.Text, and drops empty results so the contract of
// mdtext.SplitSentences (returns a non-empty []string with
// TrimSpace'd entries) is preserved. dst is appended to so callers
// can pool it across calls — SplitSentences passes nil and
// SplitSentencesInto forwards the caller's dst.
func splitSentencesInto(dst []string, text string) []string {
	sents := forkTokenizer.Tokenize(text)
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
