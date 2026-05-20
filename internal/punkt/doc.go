// Package punkt is a forked, allocation-clean subset of the trained
// Punkt sentence tokenizer from neurosnap/sentences v1.1.2
// (https://github.com/neurosnap/sentences). It vendors only what
// MDS024 needs — Storage, Token, WordTokenizer, TokenGrouper,
// OrthoContext, DefaultSentenceTokenizer, and the English supervised
// abbreviations — and drops CJK punctuation, the non-English language
// data, and IsNonPunct (no call site in upstream's English pipeline,
// per plan 187).
//
// The fork is segmentation-equivalent to upstream over the
// equivalence corpus in internal/mdtext/sentence_equivalence_test.go
// — that is the gate any drift fails on. The differences from
// upstream are all allocation-driven:
//
//   - Token carries no regex pointers; the upstream per-token
//     allocation of six *regexp.Regexp is gone. Type-classification
//     regexes (reInitial, reAlpha, reEllipsis, reListNumber,
//     reCoordinateSecondPart, reNumeric) are replaced with byte
//     scanners at package scope.
//   - WordTokenizer.Type is a one-pass byte scan into a reusable
//     buffer instead of `reNumeric.ReplaceAllString` +
//     `strings.ToLower` + `strings.Replace`.
//   - Collocation lookups rebuild the upstream `typ + "," + nextTyp`
//     key into a reusable byte buffer and hit
//     `Collocations[string(buf)]` — relying on the compiler's
//     `m[string(b)]` elision so the lookup itself does not
//     allocate, instead of `strings.Join` followed by a SetString
//     lookup.
//   - TokenGrouper reuses a buffer across passes; one
//     allocation per Tokenize call instead of three.
//   - TypeBasedAnnotation's hyphenation check is a
//     bytes.IndexByte-driven scan over the suffix, replacing
//     strings.Split.
//   - The hot per-call buffers (tokens, ptrs, pairs, type-builder
//     bytes) come from a sync.Pool so a sequence of Tokenize calls
//     amortizes their allocations to ~0.
//
// Plan 193 records the rationale and the per-call allocation budget.
//
// Upstream commit: github.com/neurosnap/sentences@v1.1.2
// (https://github.com/neurosnap/sentences/tree/v1.1.2).
// License: MIT — see the UPSTREAM_LICENSE file in this package for
// the verbatim upstream copyright and permissions notice. (The file
// has no extension so mdsmith's content rules do not lint the
// verbatim license text.)
//
// CJK punctuation, multilingual loaders, and IsNonPunct are not
// vendored. Re-adding any of them must run the equivalence harness;
// upstream's CJK code paths have never been exercised by mdsmith.
package punkt
