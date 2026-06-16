package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refMap reduces a slice of references to a normalised-label -> (dest,
// title) map, so two reference sets can be compared independent of slice
// order (goldmark's References() iterates a map, so order is undefined).
func refMap(refs []Reference) map[string][2]string {
	out := make(map[string][2]string, len(refs))
	for _, r := range refs {
		out[util.ToLinkReference(r.Label())] = [2]string{
			string(r.Destination()), string(r.Title()),
		}
	}
	return out
}

// astRefs returns the reference map a full goldmark parse produces for
// src — the ground truth the byte-level scanner must match.
func astRefs(t *testing.T, src []byte) map[string][2]string {
	t.Helper()
	f, err := NewFile("t.md", src)
	require.NoError(t, err)
	return refMap(f.LinkReferences())
}

func TestScanLinkReferences_Equivalence(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"single", "[a]: /a\n"},
		{"multiple-head", "[a]: /a\n[b]: /b\n[c]: /c\n"},
		{"defs-then-prose", "[a]: /a\n[b]: /b\n\nSome [a] prose.\n"},
		{"prose-then-defs", "Intro paragraph.\n\n[a]: /a\n[b]: /b\n"},
		{"title-double", "[a]: /a \"title\"\n"},
		{"title-paren", "[a]: /a (title)\n"},
		{"angle-dest", "[a]: <http://example.com/x>\n"},
		{"case-fold", "[FooBar]: /x\n\nuse [foobar].\n"},
		{"title-next-line", "[a]: /a\n   \"the title\"\n"},
		{"dup-first-wins", "[a]: /first\n\n[a]: /second\n"},
		{"no-defs", "Just a paragraph with [a](inline) link.\n"},
		{"def-in-code-fence", "```\n[a]: /a\n```\n"},
		{"def-after-heading", "# Title\n\n[a]: /a\n"},
		{"multiline-label", "[a\nb]: /ab\n\nuse [a b].\n"},
		{"def-not-at-head", "Text line.\n[a]: /a\n"},
		// A definition directly above a setext underline: goldmark peels
		// the definition off before the underline forms a heading, so the
		// reference is real. Layer 0 flattens the pair into one
		// BlockSetextHeading span, which scanLinkReferences must still feed
		// to the parser. See the setext branch in scanLinkReferences.
		{"def-then-setext-equals", "[a]: /a\n===\n"},
		{"def-then-setext-dash", "[a]: /a\n---\n"},
		{"def-then-setext-use", "[ref]: x.png\n===\n\nuse [ref].\n"},
		{"two-defs-then-setext", "[a]: /a\n[b]: /b\n===\n"},
		{"empty", ""},
		{"crlf", "[a]: /a\r\n"},
		{"no-trailing-newline", "[a]: /a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			want := astRefs(t, src)
			f := NewFileLines("t.md", src)
			got := refMap(scanLinkReferences(f))
			assert.Equal(t, want, got)
		})
	}
}

// TestScanLinkReferences_FallbackContainer checks that cases the byte
// scanner cannot handle correctly trigger the full-parse fallback so the
// result still matches the AST. Two distinct reasons: block quotes and
// loose list items hold definitions in a container the paragraph-head
// scanner never visits; tight list continuations produce a false positive
// (the scanner sees the continuation as a depth-0 paragraph) that
// scanNeedsFallback suppresses by detecting the span immediately follows
// a BlockList span.
func TestScanLinkReferences_FallbackContainer(t *testing.T) {
	cases := []string{
		"> [a]: /a\n",
		"- item\n\n  [a]: /a\n",
		"- item\n  [a]: /a\n",
	}
	for _, src := range cases {
		want := astRefs(t, []byte(src))
		f := NewFileLines("t.md", []byte(src))
		// LinkReferences (not scanLinkReferences) must take the fallback
		// and still match the AST result.
		got := refMap(f.LinkReferences())
		assert.Equal(t, want, got, "src=%q", src)
	}
}

// TestLinkReferences_NilASTUsesScanner pins that a File built without an
// AST (the parse-skipped path) returns the same references as the AST
// path, via the byte-level scanner.
func TestLinkReferences_NilASTUsesScanner(t *testing.T) {
	src := []byte("See [a] and [b].\n\n[a]: https://example.com/a\n[B]: https://example.com/b\n")
	want := astRefs(t, src)
	f := NewFileLines("t.md", src)
	require.Nil(t, f.AST)
	got := refMap(f.LinkReferences())
	assert.Equal(t, want, got)
}

// TestScanLinkReferences_CorpusEquivalence walks the repository's own
// Markdown and asserts the byte-level scanner's reference map is
// byte-identical to the full goldmark parse for every file. It runs
// everywhere (no env gate) because the repo Markdown is checked in.
func TestScanLinkReferences_CorpusEquivalence(t *testing.T) {
	root := repoRoot(t)
	var files, diverged int
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(p) != ".md" {
			return nil
		}
		src, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Errorf("read %s: %v", p, readErr)
			return nil
		}
		files++
		af, parseErr := NewFile(p, src)
		if parseErr != nil {
			t.Errorf("parse %s: %v", p, parseErr)
			return nil
		}
		want := refMap(af.LinkReferences())
		f := NewFileLines(p, src)
		got := refMap(f.LinkReferences())
		if !assert.ObjectsAreEqual(want, got) {
			diverged++
			if diverged <= 10 {
				t.Errorf("%s: scanner refs diverge\n want=%v\n  got=%v",
					p, want, got)
			}
		}
		return nil
	})
	t.Logf("corpus link-ref equivalence: files=%d diverged=%d", files, diverged)
	assert.Zero(t, diverged)
}

// repoRoot walks up from the test's working directory to the module
// root (the directory holding go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test working dir")
		}
		dir = parent
	}
}
