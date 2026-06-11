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
	pooled, release, err := NewFileFromSourcePooled("doc.md", []byte(pooledFixture), true)
	require.NoError(t, err)
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
	f1, release1, err := NewFileFromSourcePooled("a.md", []byte("# First\n\nAlpha beta.\n"), false)
	require.NoError(t, err)
	kinds1 := walkKinds(t, f1)
	release1()

	f2, release2, err := NewFileFromSourcePooled("b.md", []byte("- one\n- two\n\n> quote\n"), false)
	require.NoError(t, err)
	defer release2()
	kinds2 := walkKinds(t, f2)

	assert.Contains(t, kinds1, "Heading")
	assert.Contains(t, kinds2, "List")
	assert.Contains(t, kinds2, "Blockquote")
	assert.NotEqual(t, kinds1, kinds2)
}

func TestNewFileFromSourcePooled_ReleaseIdempotent(t *testing.T) {
	_, release, err := NewFileFromSourcePooled("a.md", []byte("# H\n"), false)
	require.NoError(t, err)
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
		f, release, err := NewFileFromSourcePooled("doc.md", []byte(doc), false)
		require.NoError(t, err)
		plain, err := NewFileFromSource("doc.md", []byte(doc), false)
		require.NoError(t, err)
		assert.Equal(t, walkKinds(t, plain), walkKinds(t, f), "cycle %d", i)
		release()
	}
}
