package lint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbeAdv_CodeSpan(t *testing.T) {
	cases := map[string]string{
		// HTML block that goldmark continues across a "blank-looking" boundary
		"html-block-span-inside": "<div>\n`a`\n\n`b`\n</div>\n\n`real`\n",
		"html-comment-block":     "<!-- `a` -->\n\n`real`\n",
		"html-block-no-close":    "<div>\n`a` `b`\n",
		// lazy continuation: paragraph continues a list item without blank
		"lazy-cont-span": "- item one\nlazy `a` cont\n",
		// indented code interrupting
		"indented-code-between": "`a`\n\n    indented `not`\n\n`b`\n",
		// setext heading where opener line has a span
		"setext-span-multiline": "text `a\nb` more\n===\n",
		// blockquote lazy continuation
		"bq-lazy-span": "> quote `a`\nlazy `b`\n",
		// span right before a fenced block (run ends)
		"span-before-fence": "`a`\n```\nx\n```\n",
		// tilde fence
		"tilde-fence-span": "`a`\n~~~\nx\n~~~\n\n`b`\n",
		// HTML inline (not block) with span
		"inline-html-span": "text <span>x</span> `a`\n",
		// multiple HTML blocks separating spans
		"two-html-blocks": "`a`\n\n<div>x</div>\n\n`b`\n\n<p>y</p>\n\n`c`\n",
		// nested blockquote + list + span continuation
		"bq-list-cont-span": "> - item\n>   `a\n>   b`\n",
		// reference def line in middle of doc with span before
		"span-refdef-span": "`a` text\n\n[r]: /x\n\n`b` text\n",
		// trailing span no newline after fence-less
		"span-eof-bare": "just `code`",
		// tab-indented code block
		"tab-code-block": "`a`\n\n\t`not`\n\n`b`\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			astFile, err := NewFile("d.md", []byte(src))
			require.NoError(t, err)
			lineFile := NewFileLines("d.md", []byte(src))
			ac := astFile.CodeSpanContentRanges()
			lc := lineFile.CodeSpanContentRanges()
			al := astFile.CodeSpanLiteralRanges()
			ll := lineFile.CodeSpanLiteralRanges()
			if !rangesEqual(ac, lc) {
				t.Errorf("CONTENT diverge\n src=%q\n ast=%v\n l1 =%v", src, ac, lc)
			}
			if !rangesEqual(al, ll) {
				t.Errorf("LITERAL diverge\n src=%q\n ast=%v\n l1 =%v", src, al, ll)
			}
		})
	}
}
