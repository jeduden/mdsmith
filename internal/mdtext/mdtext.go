package mdtext

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
)

// Slugify converts heading text to a GitHub-compatible URL anchor slug.
// Lowercase, letters/digits preserved, spaces and hyphens become a single dash.
func Slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// TOCItem represents a single heading entry for table-of-contents generation.
type TOCItem struct {
	Level  int
	Text   string
	Anchor string
}

// CollectTOCItems returns all headings from the AST as TOC items, in document
// order. Anchors are disambiguated by insertion order: first occurrence keeps
// the plain slug, subsequent duplicates get -1, -2, … suffixes — matching the
// anchor computation in crossfilereferenceintegrity. Tracks used anchors (not
// just base slugs) to guarantee unique anchors even when a later heading's
// base slug matches an earlier heading's disambiguated anchor.
func CollectTOCItems(root ast.Node, source []byte) []TOCItem {
	var items []TOCItem
	usedAnchors := make(map[string]bool)
	slugCounts := make(map[string]int)
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := ExtractPlainText(h, source)
		slug := Slugify(text)
		if slug == "" {
			return ast.WalkContinue, nil
		}

		// Find a unique anchor by incrementing suffix until unused.
		anchor := slug
		if usedAnchors[anchor] {
			count := slugCounts[slug]
			for {
				count++
				anchor = fmt.Sprintf("%s-%d", slug, count)
				if !usedAnchors[anchor] {
					break
				}
			}
			slugCounts[slug] = count
		}

		usedAnchors[anchor] = true
		items = append(items, TOCItem{Level: h.Level, Text: text, Anchor: anchor})
		return ast.WalkContinue, nil
	})
	return items
}

// extractTextBufPool reuses bytes.Buffer backing across
// ExtractPlainText calls. strings.Builder cannot be pooled because
// its Reset() nils the backing slice (the unsafe.String trick in
// String() ties the result string to the backing memory), so each
// reset-then-write call has to allocate again. bytes.Buffer's
// Reset() preserves the backing slice and zeroes its length, so
// subsequent appends reuse the memory. We pay one alloc for the
// resulting string (buf.String() makes the safe copy) and nothing
// for the buffer itself after the first call into a goroutine.
//
// The pool returns buffers up to extractTextMaxPooledCap bytes.
// Past that, the buffer is discarded — a single large document
// (e.g. ExtractPlainText on a whole `f.AST` from
// internal/metrics/document.go) would otherwise inflate the
// pool's steady-state footprint forever, which matters most for
// the LSP's long-running process.
var extractTextBufPool = sync.Pool{
	New: func() any {
		b := &bytes.Buffer{}
		b.Grow(128)
		return b
	},
}

// extractTextMaxPooledCap bounds the pool slot's per-buffer
// capacity. A buffer that grew past this size is discarded on
// release rather than returned to the pool. 64 KiB covers the
// large headings and paragraphs the bench actually produces (the
// 600-file synthetic corpus tops out around 1 KiB per call)
// without retaining multi-MiB outliers from one-off
// whole-document extracts.
const extractTextMaxPooledCap = 64 * 1024

// ExtractPlainText extracts readable text from a goldmark AST node,
// stripping markdown syntax. Keeps: text content, link display text,
// emphasis inner text, image alt text, code span text.
func ExtractPlainText(node ast.Node, source []byte) string {
	buf := extractTextBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= extractTextMaxPooledCap {
			extractTextBufPool.Put(buf)
		}
		// Else: drop the buffer so the pool does not retain a
		// large backing array indefinitely (Copilot review on
		// PR #368 flagged the LSP long-process risk).
	}()
	extractText(buf, node, source)
	return buf.String()
}

func extractText(buf *bytes.Buffer, node ast.Node, source []byte) {
	// For text nodes, write the content.
	if t, ok := node.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		if t.SoftLineBreak() || t.HardLineBreak() {
			buf.WriteByte(' ')
		}
		return
	}

	// For string nodes (emitted by typographer / smart-quote /
	// auto-link extensions and some paragraph transformers), the
	// payload lives on the node, not in the source buffer. Without
	// this branch, ExtractPlainText silently drops them — and any
	// heading whose text was rewritten by such an extension would
	// produce a blank slug or an incorrect anchor.
	if s, ok := node.(*ast.String); ok {
		buf.Write(s.Value)
		return
	}

	// For code spans, extract the text content from children.
	if _, ok := node.(*ast.CodeSpan); ok {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				buf.Write(t.Segment.Value(source))
			}
		}
		return
	}

	// For images, use alt text from children.
	if _, ok := node.(*ast.Image); ok {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			extractText(buf, c, source)
		}
		return
	}

	// For links, emphasis, strong, and other nodes: recurse into children.
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		extractText(buf, c, source)
	}
}

// IsSpace reports whether r is a Unicode space, with exactly the
// result unicode.IsSpace gives but an inlinable ASCII fast path: for
// r < utf8.RuneSelf the only spaces are ' ' and '\t'..'\r', so two
// integer comparisons decide it and only genuine non-ASCII runes pay
// for unicode.IsSpace's table lookup. It is called per rune of every
// word of every file on the check hot path, where unicode.IsSpace
// alone was ~5.5% of CPU (plan 175 profiling).
func IsSpace(r rune) bool {
	if r < utf8.RuneSelf {
		return r == ' ' || ('\t' <= r && r <= '\r')
	}
	return unicode.IsSpace(r)
}

// CountWords counts whitespace-delimited words in text. It is exactly
// len(strings.Fields(text)) — a word is a maximal run of non-space
// runes, space being [IsSpace] (exactly unicode.IsSpace) — but counts
// in a single rune scan instead of allocating the []string. CountWords
// is called per sentence, per paragraph, per file; the slice
// strings.Fields built only to be discarded was ~0.48 GB over the
// 600-file check gate (plan 175 profiling).
func CountWords(text string) int {
	n := 0
	inWord := false
	for _, r := range text {
		if IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			inWord = true
			n++
		}
	}
	return n
}

// CountWordsInNode returns the word count of [ExtractPlainText](node,
// source) without materialising the joined string. A word is a maximal
// run of non-space runes (space being [IsSpace]); the boundary between
// adjacent text segments only counts as a word break if [ExtractPlainText]
// would have emitted whitespace there — i.e., it carries the inWord state
// across segments so back-to-back writes coalesce. CountWordsInNode
// must return the same count as CountWords(ExtractPlainText(node, source))
// for every input; an equivalence harness in the rule package pins this
// on every fixture paragraph.
//
// Used by paragraph-readability's minWords gate: most synthetic-corpus
// paragraphs fall below the gate and the materialised ExtractPlainText
// string is wasted. CountWordsInNode skips the allocation entirely.
//
// Precondition: node must be non-nil — passing nil panics on the
// FirstChild dereference, the same shape as [ExtractPlainText].
func CountWordsInNode(node ast.Node, source []byte) int {
	var wc wordCounter
	countWordsInNode(&wc, node, source)
	return wc.n
}

// wordCounter accumulates a word count across multiple byte segments
// while preserving the inWord state at segment boundaries — mirroring
// the way [ExtractPlainText] would have concatenated those segments
// before [CountWords] tallied the joined string.
type wordCounter struct {
	n      int
	inWord bool
}

// writeBytes folds b into the running count, decoding UTF-8 runes
// one at a time. Equivalent to feeding b through [CountWords] except
// that the inWord state is shared with later writeBytes / writeSpace
// calls so two adjacent calls do not introduce a spurious word break.
func (wc *wordCounter) writeBytes(b []byte) {
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if IsSpace(r) {
			wc.inWord = false
		} else if !wc.inWord {
			wc.inWord = true
			wc.n++
		}
		b = b[size:]
	}
}

// writeSpace marks a word boundary without writing any rune —
// matches ExtractPlainText's `buf.WriteByte(' ')` for Text nodes with
// SoftLineBreak / HardLineBreak set.
func (wc *wordCounter) writeSpace() { wc.inWord = false }

// countWordsInNode mirrors [extractText]'s dispatch shape so the two
// stay equivalent under [CountWords]. Any change to the cases below
// must also update extractText (and vice versa); the equivalence
// harness in internal/rules/paragraphreadability/ catches drift.
func countWordsInNode(wc *wordCounter, node ast.Node, source []byte) {
	if t, ok := node.(*ast.Text); ok {
		wc.writeBytes(t.Segment.Value(source))
		if t.SoftLineBreak() || t.HardLineBreak() {
			wc.writeSpace()
		}
		return
	}
	if s, ok := node.(*ast.String); ok {
		wc.writeBytes(s.Value)
		return
	}
	if _, ok := node.(*ast.CodeSpan); ok {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				wc.writeBytes(t.Segment.Value(source))
			}
		}
		return
	}
	if _, ok := node.(*ast.Image); ok {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			countWordsInNode(wc, c, source)
		}
		return
	}
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		countWordsInNode(wc, c, source)
	}
}

// CountSentences counts sentences by splitting on sentence-ending
// punctuation (., !, ?) followed by whitespace or end of text.
// Returns at least 1 for non-empty text.
func CountSentences(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	count := 0
	runes := []rune(text)
	for i, r := range runes {
		if r == '.' || r == '!' || r == '?' {
			if i == len(runes)-1 {
				count++
			} else if IsSpace(runes[i+1]) {
				count++
			}
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

// initTokenizerOnce wraps initTokenizer in sync.OnceFunc — a
// stylistic preference over `var initOnce sync.Once` /
// `initOnce.Do(initTokenizer)` at each call site, not a perf win:
// passing a package-level function value to sync.Once.Do is
// allocation-free per call (only closures and method values force
// the function-value boxing the budget gate would see). OnceFunc
// constructs the wrapper once at package init; the call site
// becomes `initTokenizerOnce()` which reads cleaner than the
// explicit Once-and-Do pair.
//
// The actual singleton is owned by the build-tagged file that
// provides splitSentences — fastpunct_init.go (default) builds a
// *punkt.Tokenizer; upstreampunct.go (tag mdtext_punkt_upstream)
// builds the upstream english.NewSentenceTokenizer pipeline. Both
// paths produce byte-identical segmentation, gated by
// sentence_equivalence_test.go.
var initTokenizerOnce = sync.OnceFunc(initTokenizer)

// SplitSentences splits text into individual sentences using a
// Punkt sentence tokenizer. Handles abbreviations, decimals, and
// ellipses. The actual segmentation is delegated to splitSentences
// (defined by the active build tag).
//
// The returned slice is freshly allocated. Hot callers that want
// to pool the destination should use SplitSentencesInto instead.
func SplitSentences(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	initTokenizerOnce()
	return splitSentencesInto(nil, text)
}

// SplitSentencesInto is the pool-friendly variant of SplitSentences:
// it appends the segmented sentences (trimmed, non-empty) to dst and
// returns the extended slice. The intended pattern is
//
//	bufPtr := sentBufPool.Get().(*[]string)
//	*bufPtr = mdtext.SplitSentencesInto((*bufPtr)[:0], text)
//	defer sentBufPool.Put(bufPtr)
//
// so the per-call `make([]string, 0, n)` plain SplitSentences pays
// is amortized across a sync.Pool. MDS024's hot path uses this form
// to stay within the per-rule allocation budget on cold-File runs.
func SplitSentencesInto(dst []string, text string) []string {
	if strings.TrimSpace(text) == "" {
		return dst
	}
	initTokenizerOnce()
	return splitSentencesInto(dst, text)
}

// CountCharacters counts letters and digits in text
// (no spaces or punctuation).
func CountCharacters(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			count++
		}
	}
	return count
}
