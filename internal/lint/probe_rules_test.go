package lint_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/linkvalidity"
	"github.com/jeduden/mdsmith/internal/rules/nobareurls"
	"github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"
	"github.com/jeduden/mdsmith/internal/rules/noemptyalttext"
)

func diagKey(d lint.Diagnostic) string {
	return string(d.Severity) + d.RuleID + d.Message + itoa(d.Line) + ":" + itoa(d.Column)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

func TestProbeRules_ASTvsNil(t *testing.T) {
	rules := []rule.Rule{
		&noemphasisasheading.Rule{},
		&nobareurls.Rule{},
		&noemptyalttext.Rule{},
		&linkvalidity.Rule{},
	}
	cases := map[string]string{
		"fm-list-emph":        "---\nt: x\n---\n\n- *a*\n\n- *b*\n",
		"fm-only-emph":        "---\nt: x\n---\n\n*lone*\n",
		"deep-nested-span":    "- a\n  - b\n    - `[ref]` here\n",
		"bq-list-span":        "> - item `code`\n> - two [ref]\n",
		"refdef-codespan-run": "use `[ref]` here\n\n[ref]: /x\n",
		"crlf-bareurl":        "see http://example.com x\r\nmore http://b.com y\r\n",
		"no-trailing-nl":      "see http://example.com here",
		"undef-ref-in-span":   "use `[undef]` not a ref\n",
		"undef-ref-real":      "use [undef] here\n",
		"reversed-link":       "(text)[http://example.com]\n",
		"empty-alt":           "![](img.png)\n",
		"loose-list-emph-fm":  "---\nt: y\n---\n\n*top*\n\n- *x*\n\n- *y*\n\n*end*\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			for _, r := range rules {
				astFile, err := lint.NewFileFromSource("doc.md", []byte(src), true)
				if err != nil {
					t.Fatal(err)
				}
				lineFile := lint.NewFileLinesFromSource("doc.md", []byte(src), true)
				ad := r.Check(astFile)
				ld := r.Check(lineFile)
				if len(ad) != len(ld) {
					t.Errorf("[%s/%s] count diverge: ast=%d nil=%d\n ast=%v\n nil=%v",
						name, r.ID(), len(ad), len(ld), ad, ld)
					continue
				}
				for i := range ad {
					if diagKey(ad[i]) != diagKey(ld[i]) {
						t.Errorf("[%s/%s] diag %d diverge: ast=%+v nil=%+v",
							name, r.ID(), i, ad[i], ld[i])
					}
				}
			}
		})
	}
}
