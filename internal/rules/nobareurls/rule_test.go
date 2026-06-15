package nobareurls

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_BareURL(t *testing.T) {
	src := []byte("Visit https://example.com for info\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Column != 7 {
		t.Errorf("expected column 7, got %d", d.Column)
	}
	if d.RuleID != "MDS012" {
		t.Errorf("expected rule ID MDS012, got %s", d.RuleID)
	}
	if d.Message != "bare URL — wrap in angle brackets or add link text" {
		t.Errorf("unexpected message: %q", d.Message)
	}
}

func TestFixTitle(t *testing.T) {
	r := &Rule{}
	if got := r.FixTitle(); got != "Wrap in angle brackets" {
		t.Errorf("unexpected fix title: %q", got)
	}
}

func TestCheck_AngleBracketLink(t *testing.T) {
	src := []byte("Visit <https://example.com> for info\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_InlineLink(t *testing.T) {
	src := []byte("Visit [example](https://example.com)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_URLInFencedCodeBlock(t *testing.T) {
	src := []byte("```\nhttps://example.com\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_URLInInlineCode(t *testing.T) {
	src := []byte("Use `https://example.com` for info\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_ReferenceDefinition(t *testing.T) {
	src := []byte("[label]: https://example.com\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_MultipleBareURLs(t *testing.T) {
	src := []byte("Visit https://example.com\nAlso see http://test.org\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d", len(diags))
	if diags[0].Line != 1 {
		t.Errorf("expected first diagnostic on line 1, got %d", diags[0].Line)
	}
	if diags[1].Line != 2 {
		t.Errorf("expected second diagnostic on line 2, got %d", diags[1].Line)
	}
}

func TestFix_WrapsBareURL(t *testing.T) {
	src := []byte("Visit https://example.com for info\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "Visit <https://example.com> for info\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_MultipleURLs(t *testing.T) {
	src := []byte("Visit https://example.com and http://test.org\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "Visit <https://example.com> and <http://test.org>\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChange(t *testing.T) {
	src := []byte("Visit [example](https://example.com)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_URLInLinkText(t *testing.T) {
	// URL appearing as the text of a link (inside []) - still inside an ast.Link
	src := []byte("[https://example.com](https://example.com)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() == "" {
		t.Error("expected non-empty category")
	}
}

// TestCheck_NilASTEquivalence pins the parse-skipped path (f.AST nil,
// served from the Layer 1 per-block inline parse) byte-identical to the
// AST path across the shapes the corpus exercises: plain prose, headings,
// list items, links and autolinks (whose URLs must not be flagged), code
// spans, and multi-line paragraphs.
func TestCheck_NilASTEquivalence(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"prose-bare", "Visit https://example.com today\n"},
		{"no-url", "just a plain paragraph\n"},
		{"linked", "See [example](https://example.com) here\n"},
		{"autolink", "See <https://example.com> here\n"},
		{"code-span-url", "Run `https://example.com` here\n"},
		{"heading-bare", "# See https://example.com\n"},
		{"list-bare", "- visit https://a.com\n- and https://b.com\n"},
		{"list-mixed", "- visit https://a.com\n- and [x](https://b.com)\n"},
		{"link-text-url", "[https://example.com](https://example.com)\n"},
		{"two-paragraphs", "first https://a.com\n\nsecond https://b.com\n"},
		{"multiline-para", "line one https://a.com\nline two https://b.com\n"},
		{"quote-bare", "> quoted https://example.com\n"},
		{"http-and-https", "http://a.com and https://b.com\n"},
		{"trailing-punct", "see https://example.com.\n"},
		{"wrapped-list-url", "- see\n  https://example.com\n"},
		{"bq-multiline-url", "> line one https://a.com\n> line two\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			astFile, err := lint.NewFile("test.md", []byte(tc.src))
			require.NoError(t, err)
			lineFile := lint.NewFileLines("test.md", []byte(tc.src))
			r := &Rule{}
			assert.Equal(t, r.Check(astFile), r.Check(lineFile),
				"nil-AST diagnostics diverge from AST path")
		})
	}
}
