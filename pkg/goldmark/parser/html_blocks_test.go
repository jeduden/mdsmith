package parser_test

// Coverage for the seven HTML-block parser types (per CommonMark
// §4.6) and the inline raw-HTML parser variants. Each block type
// is opened by a different prefix and closed by a different rule;
// driving one example per type lifts html_block.go from 26 % to
// near full coverage.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func parseWithDefaults(src string) ast.Node {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	return p.Parse(text.NewReader([]byte(src)), parser.WithContext(parser.NewContext()))
}

func TestHTMLBlock_AllSevenTypes(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"type1-script", "<script>alert('x')</script>\n"},
		{"type1-pre", "<pre>preformatted</pre>\n"},
		{"type1-style", "<style>body{}</style>\n"},
		{"type2-comment", "<!-- a comment -->\n"},
		{"type3-pi", "<?xml version=\"1.0\"?>\n"},
		{"type4-decl", "<!DOCTYPE html>\n"},
		{"type5-cdata", "<![CDATA[content]]>\n"},
		{"type6-block-tag", "<div>\nblock\n</div>\n"},
		{"type6-self-closing", "<hr />\n"},
		{"type7-block-on-its-own-line", "<a href=\"x\">\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := parseWithDefaults(tc.src)
			found := false
			_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				if entering && n.Kind() == ast.KindHTMLBlock {
					found = true
				}
				return ast.WalkContinue, nil
			})
			if !found {
				t.Errorf("expected HTMLBlock for %q", tc.src)
			}
		})
	}
}

func TestRawHTML_InlineTags(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"open-tag", "see <span class=\"x\"> inline\n"},
		{"close-tag", "see </span> inline\n"},
		{"self-closing", "see <br/> inline\n"},
		{"comment", "see <!-- skip --> here\n"},
		{"pi", "see <?xml inline?> here\n"},
		{"decl", "see <!DOCTYPE html> here\n"},
		{"cdata", "see <![CDATA[x]]> here\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := parseWithDefaults(tc.src)
			found := false
			_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				if entering && n.Kind() == ast.KindRawHTML {
					found = true
				}
				return ast.WalkContinue, nil
			})
			if !found {
				t.Errorf("expected RawHTML for %q", tc.src)
			}
		})
	}
}

func TestBlockquote_NestedAndLazy(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"single", "> one\n"},
		{"multi-line", "> one\n> two\n> three\n"},
		{"lazy-continuation", "> one\ntwo\n"},
		{"with-paragraph-inside", "> first paragraph\n>\n> second paragraph\n"},
		{"with-heading-inside", "> # heading\n"},
		{"with-code-inside", "> ```\n> code\n> ```\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := parseWithDefaults(tc.src)
			found := false
			_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				if entering && n.Kind() == ast.KindBlockquote {
					found = true
				}
				return ast.WalkContinue, nil
			})
			if !found {
				t.Errorf("expected Blockquote for %q", tc.src)
			}
		})
	}
}

func TestList_DeepNesting(t *testing.T) {
	src := `- a
  - b
    - c
      1. ordered
      2. ordered2
  - d
- e

* mixed bullet
+ another bullet
`
	root := parseWithDefaults(src)
	listCount := 0
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindList {
			listCount++
		}
		return ast.WalkContinue, nil
	})
	if listCount < 3 {
		t.Errorf("expected nested lists, got %d List nodes", listCount)
	}
}

func TestSetextHeading_EdgeCases(t *testing.T) {
	cases := []string{
		"Title\n=====\n",
		"Title\n=\n",
		"Title\n-\n",
		"Title\nSubtitle\n========\n",
	}
	for _, src := range cases {
		root := parseWithDefaults(src)
		found := false
		_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && n.Kind() == ast.KindHeading {
				found = true
			}
			return ast.WalkContinue, nil
		})
		if !found {
			t.Errorf("expected Heading for %q", src)
		}
	}
}

func TestCodeBlock_FencedWithVariations(t *testing.T) {
	cases := []string{
		"```\nbody\n```\n",
		"```go\nbody\n```\n",
		"~~~\nbody\n~~~\n",
		"~~~ python\nbody\n~~~\n",
		"  ```\n  indented fence\n  ```\n",
	}
	for _, src := range cases {
		root := parseWithDefaults(src)
		found := false
		_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && n.Kind() == ast.KindFencedCodeBlock {
				found = true
			}
			return ast.WalkContinue, nil
		})
		if !found {
			t.Errorf("expected FencedCodeBlock for %q", src)
		}
	}
}
