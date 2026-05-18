package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
)

func newFile(t *testing.T, source string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(source))
	require.NoError(t, err)
	return f
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   Target
		wantOK bool
	}{
		{"empty", "", Target{}, false},
		{"protocol-relative", "//example.com/x", Target{}, false},
		{"scheme", "https://example.com", Target{}, false},
		{"plain path", "guide.md", Target{Raw: "guide.md", Path: "guide.md"}, true},
		{"path with anchor", "guide.md#sec", Target{Raw: "guide.md#sec", Path: "guide.md", Anchor: "sec"}, true},
		{"anchor only", "#sec", Target{Raw: "#sec", Anchor: "sec", LocalAnchor: true}, true},
		{"query only rejected", "?q=1", Target{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseTarget(tc.input)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestExtractLinks_Basic(t *testing.T) {
	f := newFile(t, "# Doc\n\nSee [guide](guide.md#intro) and [home](/).\n")
	links := ExtractLinks(f)
	require.Len(t, links, 2)

	assert.Equal(t, "guide", links[0].Text)
	assert.Equal(t, "guide.md", links[0].Target.Path)
	assert.Equal(t, "intro", links[0].Target.Anchor)
	assert.False(t, links[0].Target.LocalAnchor)
	assert.Equal(t, 3, links[0].Line)
	assert.Greater(t, links[0].Column, 0)

	assert.Equal(t, "home", links[1].Text)
	assert.Equal(t, "/", links[1].Target.Path)
}

func TestExtractLinks_LocalAnchor(t *testing.T) {
	f := newFile(t, "# Doc\n\nGo [up](#top).\n")
	links := ExtractLinks(f)
	require.Len(t, links, 1)
	assert.True(t, links[0].Target.LocalAnchor)
	assert.Equal(t, "top", links[0].Target.Anchor)
}

func TestExtractLinks_SkipsReferenceStyle(t *testing.T) {
	src := "# Doc\n\nSee [other][label].\n\n[label]: other.md\n"
	f := newFile(t, src)
	links := ExtractLinks(f)
	// Reference-style links resolve via the ref map, not a URL; the
	// graph builder skips them.
	assert.Empty(t, links)
}

func TestExtractLinks_SkipsExternal(t *testing.T) {
	f := newFile(t, "# Doc\n\nSee [out](https://example.com).\n")
	links := ExtractLinks(f)
	assert.Empty(t, links)
}

func TestExtractLinks_NilFile(t *testing.T) {
	assert.Nil(t, ExtractLinks(nil))
}

func TestExtractLinks_LinesAreBodyRelative(t *testing.T) {
	source := []byte("---\ntitle: x\n---\nSee [g](g.md).\n")
	f, err := lint.NewFileFromSource("file.md", source, true)
	require.NoError(t, err)
	links := ExtractLinks(f)
	require.Len(t, links, 1)
	// Body starts after the 3-line front-matter prefix. The link is
	// on body line 1; lint rules will have f.AdjustDiagnostics add
	// f.LineOffset later, so ExtractLinks must NOT pre-apply it.
	assert.Equal(t, 1, links[0].Line)
	assert.Equal(t, 3, f.LineOffset, "front matter occupies 3 lines")
}

func TestCollectAnchors(t *testing.T) {
	f := newFile(t, "# Intro\n\n## Setup\n\n## Setup\n\n##   \n")
	anchors := CollectAnchors(f)
	assert.True(t, anchors["intro"])
	assert.True(t, anchors["setup"])
	assert.True(t, anchors["setup-1"])
	assert.False(t, anchors[""], "empty-text headings produce no slug")
}

// TestCollectAnchors_CollisionWithPreNumberedHeading guards against
// the disambiguation bug where two "Intro" headings followed by an
// "Intro-1" heading collapse onto the same `intro-1` anchor — the
// duplicate "Intro" generates `intro-1`, then the literal "Intro-1"
// heading also slugifies to `intro-1` and overwrites it. The fix
// must give each heading a globally unique anchor.
func TestCollectAnchors_CollisionWithPreNumberedHeading(t *testing.T) {
	f := newFile(t, "# Intro\n\n# Intro\n\n# Intro-1\n")
	anchors := CollectAnchors(f)
	// All three headings must appear under distinct anchors. The
	// canonical GitHub behavior is to keep numbering until no
	// collision exists, so the third heading becomes `intro-1-1`.
	assert.True(t, anchors["intro"], "first heading keeps the plain slug")
	assert.True(t, anchors["intro-1"], "second heading uses the next free suffix")
	assert.True(t, anchors["intro-1-1"], "third heading must not collide with the second")
}

func TestCollectAnchors_NilFile(t *testing.T) {
	got := CollectAnchors(nil)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestNormalizeAnchor(t *testing.T) {
	assert.Equal(t, "hello-world", NormalizeAnchor("Hello World"))
	assert.Equal(t, "section", NormalizeAnchor("Section"))
	// %20 URL-decodes to a space, which slugifies to a dash.
	assert.Equal(t, "two-words", NormalizeAnchor("Two%20Words"))
}

// TestExtractLinks_AgreesWithMDS027 confirms the linkgraph extractor
// reports the same set of links MDS027 sees when validating cross-file
// references. The agreement is the load-bearing invariant for the
// `backlinks` subcommand: a target's incoming-link set must mirror
// the outgoing-link set MDS027 walks.
func TestExtractLinks_AgreesWithMDS027(t *testing.T) {
	src := "# Doc\n\nA [one](a.md), [two](b.md#x), [three](#local), and [ref][r].\n\n[r]: r.md\n"
	f := newFile(t, src)
	links := ExtractLinks(f)

	// MDS027 sees the same three direct links (one, two, three) and
	// skips the reference-style link.
	require.Len(t, links, 3)
	texts := []string{links[0].Text, links[1].Text, links[2].Text}
	assert.Equal(t, []string{"one", "two", "three"}, texts)
}

// TestParseTarget_MalformedURL exercises the url.Parse error branch.
// A control character in the URL is invalid per RFC 3986 and makes
// url.Parse return an error.
func TestParseTarget_MalformedURL(t *testing.T) {
	// %ZZ is an invalid percent-encoded escape; url.Parse rejects it.
	_, ok := ParseTarget("guide.md%ZZ")
	assert.False(t, ok)
}

// TestCollectAnchors_AstStringHeading guards against a regression:
// when a heading's child is an *ast.String (emitted by typographer
// or smart-quote extensions), the slug must still pick up the
// string's value. mdtext.ExtractPlainText is the shared routine that
// handles this; without the dedicated branch the heading collapses
// to an empty slug and anchor lookups silently miss.
func TestCollectAnchors_AstStringHeading(t *testing.T) {
	heading := ast.NewHeading(2)
	heading.AppendChild(heading, ast.NewString([]byte("hello world")))
	root := ast.NewDocument()
	root.AppendChild(root, heading)
	f := &lint.File{AST: root, Source: nil}

	anchors := CollectAnchors(f)
	assert.True(t, anchors["hello-world"], "ast.String value should drive the slug")
}

// TestExtractLinks_LinkWithNoTextChildren covers the linkPosition
// `offset < 0` fallback. A link node without any text children yields
// firstTextOffset = -1, so linkPosition returns (1, 1).
func TestExtractLinks_LinkWithNoTextChildren(t *testing.T) {
	// Construct an AST with one bare ast.Link and no text under it.
	root := ast.NewDocument()
	para := ast.NewParagraph()
	link := ast.NewLink()
	link.Destination = []byte("a.md")
	para.AppendChild(para, link)
	root.AppendChild(root, para)
	f := &lint.File{AST: root, Source: nil}

	links := ExtractLinks(f)
	require.Len(t, links, 1)
	assert.Equal(t, 1, links[0].Line, "no-text link → linkPosition falls back to (1, 1)")
	assert.Equal(t, 1, links[0].Column)
}

// =====================================================================
// ExtractImages tests
// =====================================================================

func TestExtractImages_Basic(t *testing.T) {
	f := newFile(t, "# Doc\n\n![diagram](diagram.png)\n")
	imgs := ExtractImages(f)
	require.Len(t, imgs, 1)
	assert.Equal(t, "diagram", imgs[0].Text)
	assert.Equal(t, "diagram.png", imgs[0].Target.Path)
	assert.Equal(t, 3, imgs[0].Line)
	assert.Greater(t, imgs[0].Column, 0)
}

func TestExtractImages_SkipsExternal(t *testing.T) {
	f := newFile(t, "# Doc\n\n![logo](https://example.com/logo.png)\n")
	imgs := ExtractImages(f)
	assert.Empty(t, imgs, "external image URLs must be skipped")
}

func TestExtractImages_IncludesReferenceStyle(t *testing.T) {
	src := "# Doc\n\n![alt][img]\n\n[img]: diagram.png\n"
	f := newFile(t, src)
	imgs := ExtractImages(f)
	require.Len(t, imgs, 1, "reference-style images must be included")
	assert.Equal(t, "diagram.png", imgs[0].Target.Path)
}

func TestExtractImages_NilFile(t *testing.T) {
	assert.Nil(t, ExtractImages(nil))
}

func TestExtractImages_EmptyAlt(t *testing.T) {
	f := newFile(t, "# Doc\n\n![](logo.png)\n")
	imgs := ExtractImages(f)
	require.Len(t, imgs, 1)
	// Empty alt: no text children, linkPosition falls back to (1,1).
	assert.Equal(t, "logo.png", imgs[0].Target.Path)
}

// =====================================================================
// ExtractRefLinkTargets tests
// =====================================================================

func TestExtractRefLinkTargets_Basic(t *testing.T) {
	src := "# Doc\n\nSee [guide][ref].\n\n[ref]: guide.md\n"
	f := newFile(t, src)
	links := ExtractRefLinkTargets(f)
	require.Len(t, links, 1)
	assert.Equal(t, "guide", links[0].Text)
	assert.Equal(t, "guide.md", links[0].Target.Path)
}

func TestExtractRefLinkTargets_SkipsInlineLinks(t *testing.T) {
	src := "# Doc\n\nSee [inline](guide.md).\n"
	f := newFile(t, src)
	links := ExtractRefLinkTargets(f)
	assert.Empty(t, links, "inline links must not be returned")
}

func TestExtractRefLinkTargets_SkipsExternalDest(t *testing.T) {
	src := "# Doc\n\nSee [ext][ref].\n\n[ref]: https://example.com\n"
	f := newFile(t, src)
	links := ExtractRefLinkTargets(f)
	assert.Empty(t, links, "external reference targets must be skipped")
}

func TestExtractRefLinkTargets_NilFile(t *testing.T) {
	assert.Nil(t, ExtractRefLinkTargets(nil))
}

func TestExtractRefLinkTargets_UndefinedRefNotReturned(t *testing.T) {
	// [undefined] has no matching definition; goldmark does not
	// create an *ast.Link node for it, so ExtractRefLinkTargets
	// must return nothing.
	src := "# Doc\n\nSee [text][undefined].\n"
	f := newFile(t, src)
	links := ExtractRefLinkTargets(f)
	assert.Empty(t, links, "undefined reference must produce no Link")
}
