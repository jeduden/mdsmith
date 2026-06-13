package mdtext_test

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/pkg/goldmark"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseDoc parses markdown and returns the document root node.
func parseDoc(t *testing.T, src string) (ast.Node, []byte) {
	t.Helper()
	source := []byte(src)
	reader := text.NewReader(source)
	doc := goldmark.DefaultParser().Parse(reader)
	return doc, source
}

// parseParagraph parses markdown and returns the first Paragraph node.
func parseParagraph(t *testing.T, src string) (ast.Node, []byte) {
	t.Helper()
	source := []byte(src)
	reader := text.NewReader(source)
	doc := goldmark.DefaultParser().Parse(reader)
	var para ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := n.(*ast.Paragraph); ok {
				para = n
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, para, "no paragraph found")
	return para, source
}

func TestExtractPlainText_PlainParagraph(t *testing.T) {
	para, src := parseParagraph(t, "Hello world.\n")
	assert.Equal(t, "Hello world.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Link(t *testing.T) {
	para, src := parseParagraph(t, "Click [here](https://example.com) now.\n")
	assert.Equal(t, "Click here now.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Emphasis(t *testing.T) {
	para, src := parseParagraph(t, "This is *important* text.\n")
	assert.Equal(t, "This is important text.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Strong(t *testing.T) {
	para, src := parseParagraph(t, "This is **bold** text.\n")
	assert.Equal(t, "This is bold text.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_CodeSpan(t *testing.T) {
	para, src := parseParagraph(t, "Use `fmt.Println` to print.\n")
	assert.Equal(t, "Use fmt.Println to print.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Image(t *testing.T) {
	para, src := parseParagraph(t, "See ![alt text](image.png) here.\n")
	assert.Equal(t, "See alt text here.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_NestedMarkup(t *testing.T) {
	para, src := parseParagraph(
		t,
		"Click [**bold link**](https://example.com) now.\n",
	)
	assert.Equal(t, "Click bold link now.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_SoftLineBreak(t *testing.T) {
	para, src := parseParagraph(t, "Hello\nworld.\n")
	assert.Equal(t, "Hello world.", mdtext.ExtractPlainText(para, src))
}

// TestExtractPlainText_AstString covers heading/paragraph children
// emitted as *ast.String — the goldmark typographer extension and
// other inline transformers replace text segments with String nodes
// whose value lives in the node rather than the source buffer.
// Without explicit handling, ExtractPlainText drops them, which
// silently breaks anchor slugs and link-text extraction for any
// heading or link processed by such an extension.
func TestExtractPlainText_AstString(t *testing.T) {
	para := ast.NewParagraph()
	para.AppendChild(para, ast.NewString([]byte("hello world")))
	assert.Equal(t, "hello world", mdtext.ExtractPlainText(para, nil))
}

// --- CountWordsInNode tests ---

// TestCountWordsInNode pins the AST-walking word counter to its
// definition: CountWords(ExtractPlainText(node, source)) for every
// case the extractText switch handles. The cases below mirror the
// TestExtractPlainText_* set so a change to one switch arm fails
// here too. The equivalence harness in the paragraphreadability
// package widens this to every fixture paragraph; the cases below
// are the per-arm unit gate.
func TestCountWordsInNode(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{name: "plain Text", src: "Hello world.\n", want: 2},
		{name: "Link display text", src: "Click [here](https://example.com) now.\n", want: 3},
		{name: "Emphasis", src: "This is *important* text.\n", want: 4},
		{name: "Strong", src: "This is **bold** text.\n", want: 4},
		{name: "CodeSpan keeps its content as one word",
			src: "Use `fmt.Println` to print.\n", want: 4},
		{name: "Image alt text", src: "See ![alt text](image.png) here.\n", want: 4},
		{name: "nested emphasis inside link",
			src: "Click [**bold link**](https://example.com) now.\n", want: 4},
		{name: "SoftLineBreak counts as space",
			src: "Hello\nworld.\n", want: 2},
		{name: "HardLineBreak counts as space",
			src: "Hello  \nworld.\n", want: 2},
		{name: "Heading is walked like any other parent",
			src: "# Hello world\n", want: 2},
		// Non-ASCII text exercises writeBytes' multibyte decode path:
		// accented letters are non-space runes...
		{name: "multibyte non-ASCII words",
			src: "café ünïcode wörds\n", want: 3},
		// ...and a non-ASCII space (U+00A0 no-break space) is a word
		// boundary on that same decode path.
		{name: "non-ASCII space separates words",
			src: "alpha beta\n", want: 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, src := parseDoc(t, c.src)
			got := mdtext.CountWordsInNode(root, src)
			require.Equal(t, c.want, got,
				"CountWordsInNode disagrees with the documented count")
			// Tied to the existing chain so future drift between the
			// AST walker and ExtractPlainText is caught at the test
			// boundary rather than the integration harness.
			want := mdtext.CountWords(mdtext.ExtractPlainText(root, src))
			assert.Equal(t, want, got,
				"CountWordsInNode must equal CountWords(ExtractPlainText(...))")
		})
	}
}

// TestCountWordsInNode_AstString covers the *ast.String branch, which
// the parser never emits on its own. Mirrors TestExtractPlainText_AstString
// — a node tree built by hand rather than parsed, so an extension that
// rewrites a Text into an ast.String still counts toward the word
// total.
func TestCountWordsInNode_AstString(t *testing.T) {
	para := ast.NewParagraph()
	para.AppendChild(para, ast.NewString([]byte("hello world")))
	assert.Equal(t, 2, mdtext.CountWordsInNode(para, nil))
}

// TestCountWordsInNode_CoalescesAdjacentSegments pins the
// boundary-state invariant: two adjacent child writes whose joined
// run has no whitespace must count as ONE word, not two. The case
// `"foo` + `bar"` (Text("foo") immediately followed by Text("bar")
// with no SoftLineBreak between them) is exactly what code spans
// and emphasis produce, and what CountWords would tally on the
// joined "foobar".
func TestCountWordsInNode_CoalescesAdjacentSegments(t *testing.T) {
	src := []byte("foobar")
	para := ast.NewParagraph()
	t1 := ast.NewText()
	t1.Segment = text.NewSegment(0, 3) // "foo"
	t2 := ast.NewText()
	t2.Segment = text.NewSegment(3, 6) // "bar"
	para.AppendChild(para, t1)
	para.AppendChild(para, t2)
	// Mirror behavior: extractText writes "foo" then "bar" — one word
	// after concatenation. CountWordsInNode must produce the same.
	require.Equal(t, 1, mdtext.CountWordsInNode(para, src))
}

// --- CountWords tests ---

func TestCountWords_Simple(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountWords("hello world"))
}

func TestCountWords_Empty(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountWords(""))
}

func TestCountWords_MultipleSpaces(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountWords("  hello   world  "))
}

// TestCountWords_EquivalentToStringsFields pins the allocation-free
// rewrite to its original definition (len(strings.Fields(s))) across
// the whitespace shapes that matter: tabs, newlines, CRLF, leading and
// trailing runs, Unicode spaces (NBSP, ideographic space), and CJK
// runs with no ASCII space. If these ever diverge, the rewrite changed
// behaviour and the rule output would shift.
func TestCountWords_EquivalentToStringsFields(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"\t\n\r ",
		"one",
		"hello world",
		"  hello   world  ",
		"a\tb\nc\r\nd",
		"line one\nline two\n",
		"emoji 🚀 done",
		"non breaking space",
		"ideographic　space　here",
		"日本語 と English",
		"trailing ",
		" leading",
	}
	for _, s := range cases {
		assert.Equalf(t, len(strings.Fields(s)), mdtext.CountWords(s),
			"CountWords must equal len(strings.Fields(%q))", s)
	}
}

// --- CountSentences tests ---

func TestCountWordsBytes_EquivalentToCountWords(t *testing.T) {
	cases := []string{
		"",
		"one",
		"two words",
		"  leading and trailing  ",
		"tabs\tand\nnewlines",
		"non breaking space", // U+00A0 is a Unicode space
		"ünïcode wörds here", // U+2003 em space separates words
		"　ideographic　space", // U+3000 space delimits
		"mixed ascii  and line-sep",
	}
	for _, tc := range cases {
		assert.Equal(t, mdtext.CountWords(tc), mdtext.CountWordsBytes([]byte(tc)),
			"CountWordsBytes must agree with CountWords for %q", tc)
	}
	assert.Equal(t, 2, mdtext.CountWordsBytes([]byte("hello world")))
	assert.Equal(t, 0, mdtext.CountWordsBytes(nil))
}

func TestCountSentences_OneSentence(t *testing.T) {
	assert.Equal(t, 1, mdtext.CountSentences("Hello world."))
}

func TestCountSentences_TwoSentences(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountSentences("Hello world. How are you?"))
}

func TestCountSentences_NoTerminator(t *testing.T) {
	assert.Equal(t, 1, mdtext.CountSentences("Hello world"))
}

func TestCountSentences_Empty(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountSentences(""))
}

func TestCountSentences_Exclamation(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountSentences("Wow! Amazing!"))
}

func TestCountSentences_AbbreviationNotCounted(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountSentences("Use e.g. this one."))
}

// --- SplitSentences tests ---

func TestSplitSentences_Simple(t *testing.T) {
	got := mdtext.SplitSentences("Hello world. How are you?")
	require.Len(t, got, 2)
	assert.Equal(t, "Hello world.", got[0])
	assert.Equal(t, "How are you?", got[1])
}

func TestSplitSentences_Exclamation(t *testing.T) {
	got := mdtext.SplitSentences("Wow! Amazing!")
	require.Len(t, got, 2)
}

func TestSplitSentences_Abbreviation(t *testing.T) {
	got := mdtext.SplitSentences("Dr. Smith went home.")
	require.Len(t, got, 1)
}

func TestSplitSentences_Decimal(t *testing.T) {
	got := mdtext.SplitSentences("The value is 3.14 today.")
	require.Len(t, got, 1)
}

func TestSplitSentences_Empty(t *testing.T) {
	got := mdtext.SplitSentences("")
	require.Empty(t, got)
}

func TestSplitSentences_WhitespaceOnly(t *testing.T) {
	got := mdtext.SplitSentences("   ")
	require.Empty(t, got)
}

// TestSplitSentencesInto covers both branches of the pool-friendly
// variant: the empty / whitespace-only short-circuit (returns dst
// unchanged so callers can keep a pooled []string) and the normal
// path (delegates to splitSentencesInto, which appends trimmed
// sentences). The branches were uncovered when SplitSentencesInto
// was added — see PR #367 review feedback.

func TestSplitSentencesInto_EmptyReturnsDstUnchanged(t *testing.T) {
	// Caller passes a pooled slice with prior capacity. The
	// short-circuit on whitespace-only input must NOT modify it
	// (no allocation, no truncation).
	dst := make([]string, 0, 4)
	got := mdtext.SplitSentencesInto(dst, "")
	require.Empty(t, got, "empty input must produce no sentences")
	require.Equal(t, 4, cap(got),
		"empty input must return the caller's slice unchanged "+
			"so the pool keeps its grown capacity")
}

func TestSplitSentencesInto_WhitespaceOnlyReturnsDstUnchanged(t *testing.T) {
	dst := make([]string, 0, 4)
	got := mdtext.SplitSentencesInto(dst, "   \n\t  ")
	require.Empty(t, got)
	require.Equal(t, 4, cap(got),
		"whitespace-only input must also return dst unchanged")
}

func TestSplitSentencesInto_AppendsToProvidedSlice(t *testing.T) {
	dst := make([]string, 0, 8)
	got := mdtext.SplitSentencesInto(dst, "Hello world. How are you?")
	require.Len(t, got, 2,
		"normal input must segment via the trained pipeline")
	assert.Equal(t, "Hello world.", got[0])
	assert.Equal(t, "How are you?", got[1])
}

func TestSplitSentencesInto_ReusesDstCapacity(t *testing.T) {
	// First call grows dst to fit two sentences. A second call into
	// the same backing array (after the caller reset len to 0)
	// must reuse the capacity instead of growing again.
	var dst []string
	dst = mdtext.SplitSentencesInto(dst, "First sentence. Second sentence.")
	require.Len(t, dst, 2)
	c0 := cap(dst)

	dst = mdtext.SplitSentencesInto(dst[:0], "One. Two.")
	require.Len(t, dst, 2)
	assert.GreaterOrEqual(t, cap(dst), c0,
		"second call must reuse (or grow from) the existing capacity, "+
			"not allocate a brand new backing array")
}

// --- CountCharacters tests ---

func TestCountCharacters_Simple(t *testing.T) {
	assert.Equal(t, 10, mdtext.CountCharacters("Hello, world!"))
}

func TestCountCharacters_WithDigits(t *testing.T) {
	assert.Equal(t, 6, mdtext.CountCharacters("abc 123"))
}

func TestCountCharacters_Empty(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountCharacters(""))
}

func TestCountCharacters_OnlyPunctuation(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountCharacters("...!!!"))
}

// --- Slugify tests ---

func TestSlugify_Simple(t *testing.T) {
	assert.Equal(t, "hello-world", mdtext.Slugify("Hello World"))
}

func TestSlugify_Empty(t *testing.T) {
	assert.Equal(t, "", mdtext.Slugify(""))
}

func TestSlugify_TrimSpaces(t *testing.T) {
	assert.Equal(t, "hello", mdtext.Slugify("  Hello  "))
}

func TestSlugify_SpecialChars(t *testing.T) {
	assert.Equal(t, "hello-world", mdtext.Slugify("Hello, World!"))
}

func TestSlugify_MultipleDashes(t *testing.T) {
	assert.Equal(t, "hello-world", mdtext.Slugify("Hello  World"))
}

func TestSlugify_Underscores(t *testing.T) {
	assert.Equal(t, "foo-bar", mdtext.Slugify("foo_bar"))
}

func TestSlugify_LeadingTrailingDashes(t *testing.T) {
	assert.Equal(t, "hello", mdtext.Slugify("---hello---"))
}

func TestSlugify_Unicode(t *testing.T) {
	// Unicode letters are preserved.
	result := mdtext.Slugify("Ångström")
	assert.NotEmpty(t, result)
}

// --- CollectTOCItems tests ---

func TestCollectTOCItems_Basic(t *testing.T) {
	doc, src := parseDoc(t, "# Title\n\n## Section\n\n### Sub\n")
	items := mdtext.CollectTOCItems(doc, src)
	require.Len(t, items, 3)
	assert.Equal(t, 1, items[0].Level)
	assert.Equal(t, "Title", items[0].Text)
	assert.Equal(t, "title", items[0].Anchor)
	assert.Equal(t, 2, items[1].Level)
	assert.Equal(t, "section", items[1].Anchor)
	assert.Equal(t, 3, items[2].Level)
	assert.Equal(t, "sub", items[2].Anchor)
}

func TestCollectTOCItems_DuplicateAnchors(t *testing.T) {
	doc, src := parseDoc(t, "## Foo\n\n## Foo\n\n## Foo\n")
	items := mdtext.CollectTOCItems(doc, src)
	require.Len(t, items, 3)
	assert.Equal(t, "foo", items[0].Anchor)
	assert.Equal(t, "foo-1", items[1].Anchor)
	assert.Equal(t, "foo-2", items[2].Anchor)
}

func TestCollectTOCItems_Empty(t *testing.T) {
	doc, src := parseDoc(t, "Just a paragraph.\n")
	items := mdtext.CollectTOCItems(doc, src)
	assert.Empty(t, items)
}

func TestCollectTOCItems_SuffixCollision(t *testing.T) {
	// "Foo", then "Foo-1" (explicit), then another "Foo": the auto-generated
	// "foo-1" is already taken, so the second duplicate must become "foo-2".
	doc, src := parseDoc(t, "## Foo\n\n## Foo-1\n\n## Foo\n")
	items := mdtext.CollectTOCItems(doc, src)
	require.Len(t, items, 3)
	assert.Equal(t, "foo", items[0].Anchor)
	assert.Equal(t, "foo-1", items[1].Anchor)
	assert.Equal(t, "foo-2", items[2].Anchor)
}

func TestCollectTOCItems_HeadingWithEmptySlug(t *testing.T) {
	// A heading consisting only of punctuation produces an empty slug and
	// is skipped.
	doc, src := parseDoc(t, "## ---\n\n## Normal\n")
	items := mdtext.CollectTOCItems(doc, src)
	require.Len(t, items, 1, "heading with empty slug should be skipped")
	assert.Equal(t, "Normal", items[0].Text)
}
