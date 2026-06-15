package lint

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// walkKinds renders the (kind, entering) visit sequence of a file's
// AST so two parses can be compared structurally.
func walkKinds(t *testing.T, f *File) []string {
	t.Helper()
	var seq []string
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			seq = append(seq, n.Kind().String())
		}
		return ast.WalkContinue, nil
	})
	return seq
}

const pooledFixture = "---\ntitle: T\n---\n# H1\n\nPara with [link](u) and `code`.\n\n- a\n- b\n\n```go\nfenced\n```\n"

func TestNewFileFromSourcePooled_EquivalentToUnpooled(t *testing.T) {
	plain, err := NewFileFromSource("doc.md", []byte(pooledFixture), true)
	require.NoError(t, err)
	pooled, release := NewFileFromSourcePooled("doc.md", []byte(pooledFixture), true)
	defer release()

	assert.Equal(t, plain.LineOffset, pooled.LineOffset)
	assert.Equal(t, string(plain.FrontMatter), string(pooled.FrontMatter))
	assert.Equal(t, len(plain.Lines), len(pooled.Lines))
	assert.Equal(t, walkKinds(t, plain), walkKinds(t, pooled))
	assert.Equal(t, len(plain.LinkReferences()), len(pooled.LinkReferences()))
}

func TestNewFileFromSourcePooled_ReuseAfterRelease(t *testing.T) {
	// Parse, extract everything we need as copies, release, parse the
	// next file: the second parse must be correct and the extracted
	// values must stay intact (they must not alias arena memory).
	f1, release1 := NewFileFromSourcePooled("a.md", []byte("# First\n\nAlpha beta.\n"), false)
	kinds1 := walkKinds(t, f1)
	release1()

	f2, release2 := NewFileFromSourcePooled("b.md", []byte("- one\n- two\n\n> quote\n"), false)
	defer release2()
	kinds2 := walkKinds(t, f2)

	assert.Contains(t, kinds1, "Heading")
	assert.Contains(t, kinds2, "List")
	assert.Contains(t, kinds2, "Blockquote")
	assert.NotEqual(t, kinds1, kinds2)
}

func TestNewFileFromSourcePooled_ReleaseIdempotent(t *testing.T) {
	_, release := NewFileFromSourcePooled("a.md", []byte("# H\n"), false)
	release()
	assert.NotPanics(t, release, "second release must be a no-op")
}

func TestNewFileFromSourcePooled_ManySequentialParsesStayCorrect(t *testing.T) {
	// Drive enough pooled cycles that slabs are certainly being
	// reused, asserting structural correctness each time.
	docs := []string{
		"# A\n\npara one with `span` text.\n",
		"## B\n\n- item\n- item two [l](u)\n",
		"### C\n\n> quoted\n\n```sh\nx\n```\n",
	}
	for i := 0; i < 50; i++ {
		doc := docs[i%len(docs)]
		f, release := NewFileFromSourcePooled("doc.md", []byte(doc), false)
		plain, err := NewFileFromSource("doc.md", []byte(doc), false)
		require.NoError(t, err)
		assert.Equal(t, walkKinds(t, plain), walkKinds(t, f), "cycle %d", i)
		release()
	}
}

// TestNewFileBlockOnlyPooled_SuppressesInlineKeepsBlocks covers the
// lazy-parse spike's block-only constructor: front matter is stripped and
// the offset computed (stripFrontMatter=true), the block tree is built,
// but no inline nodes are — yet link reference definitions still survive.
func TestNewFileBlockOnlyPooled_SuppressesInlineKeepsBlocks(t *testing.T) {
	src := []byte("---\ntitle: T\n---\n# H1\n\nPara with [link](u) and `code`.\n\n[link]: http://example.com\n")
	f, release := NewFileBlockOnlyPooled("doc.md", src, true)
	defer release()

	assert.Equal(t, "---\ntitle: T\n---\n", string(f.FrontMatter))
	assert.Positive(t, f.LineOffset)
	assert.True(t, f.StripFrontMatter)

	kinds := walkKinds(t, f)
	assert.Contains(t, kinds, "Heading")
	assert.Contains(t, kinds, "Paragraph")
	// Block-only: the inline phase never runs, so no inline nodes exist.
	assert.NotContains(t, kinds, "Text")
	assert.NotContains(t, kinds, "Link")
	assert.NotContains(t, kinds, "CodeSpan")
	// Link reference definitions are collected during block close.
	assert.NotEmpty(t, f.LinkReferences())
}

// TestNewFileBlockOnlyPooled_NoFrontMatterReleaseIdempotent covers the
// stripFrontMatter=false path and the idempotent release closure.
func TestNewFileBlockOnlyPooled_NoFrontMatterReleaseIdempotent(t *testing.T) {
	f, release := NewFileBlockOnlyPooled("a.md", []byte("# H\n\nBody.\n"), false)
	assert.Empty(t, f.FrontMatter)
	assert.Zero(t, f.LineOffset)
	assert.False(t, f.StripFrontMatter)
	release()
	assert.NotPanics(t, release, "second release must be a no-op")
}

// TestNewFileFlatPooled_SkipsParse proves acceptance criterion 3 at the
// File level: the flat Layer-0 constructor builds NO goldmark AST yet
// still serves a correct, AST-identical code-block line set from the flat
// classifier it attaches. Front matter is stripped and its offset recorded
// exactly as the full constructor does.
func TestNewFileFlatPooled_SkipsParse(t *testing.T) {
	src := []byte("---\ntitle: T\n---\n# H1\n\nA long line of prose.\n\n```go\nfenced code line\n```\n")
	flat, release := NewFileFlatPooled("doc.md", src, true)
	defer release()

	assert.Nil(t, flat.AST, "flat Layer-0 path must build no AST")
	assert.Equal(t, "---\ntitle: T\n---\n", string(flat.FrontMatter))
	assert.Positive(t, flat.LineOffset)
	assert.True(t, flat.StripFrontMatter)

	// CollectCodeBlockLines serves from the classifier and equals the AST
	// walk over the same stripped content.
	astFile, err := NewFileFromSource("doc.md", src, true)
	require.NoError(t, err)
	assert.Equal(t, sortedKeys(collectCodeBlockLines(astFile)), sortedKeys(CollectCodeBlockLines(flat)))
}

// TestNewFileFlatPooled_ReleaseIdempotent covers the no-op release closure.
func TestNewFileFlatPooled_ReleaseIdempotent(t *testing.T) {
	_, release := NewFileFlatPooled("a.md", []byte("# H\n"), false)
	release()
	assert.NotPanics(t, release, "second release must be a no-op")
}
