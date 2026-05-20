//go:build mdtext_punkt_upstream

package mdtext

import (
	sentlib "github.com/neurosnap/sentences"
	"github.com/neurosnap/sentences/english"
)

// buildTokenizer returns the upstream `english.NewSentenceTokenizer`
// path, with no override of the abbreviation classifier. This is the
// A/B verification path for plan 191: build with
// `-tags mdtext_punkt_upstream` to fall back to the upstream code and
// confirm that the fast-path output matches.
//
// The upstream constructor's data-load error is swallowed (same as
// the original `t, _ := english.NewSentenceTokenizer(nil)` from
// initTokenizer before plan 191) — see the comment in
// fastpunct_init.go.
func buildTokenizer() *sentlib.DefaultSentenceTokenizer {
	t, _ := english.NewSentenceTokenizer(nil)
	return t
}
