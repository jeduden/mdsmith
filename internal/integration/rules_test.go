package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/frontmatter"

	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundheadings"
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundlists"
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/tidymark/internal/rules/firstlineheading"
	_ "github.com/jeduden/tidymark/internal/rules/headingincrement"
	_ "github.com/jeduden/tidymark/internal/rules/headingstyle"
	_ "github.com/jeduden/tidymark/internal/rules/linelength"
	_ "github.com/jeduden/tidymark/internal/rules/listindent"
	_ "github.com/jeduden/tidymark/internal/rules/nobareurls"
	_ "github.com/jeduden/tidymark/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/tidymark/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/tidymark/internal/rules/nohardtabs"
	_ "github.com/jeduden/tidymark/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingspaces"
	_ "github.com/jeduden/tidymark/internal/rules/singletrailingnewline"
)

var ruleIDPattern = regexp.MustCompile(`^(TM\d+)-`)

type expectedDiag struct {
	Line    int    `yaml:"line"`
	Column  int    `yaml:"column"`
	Message string `yaml:"message"`
}

type frontMatter struct {
	Diagnostics []expectedDiag `yaml:"diagnostics"`
}

// parseFrontMatter extracts YAML front matter from markdown using
// goldmark-frontmatter, then strips it from the raw bytes so lint.NewFile
// receives plain markdown content.
func parseFrontMatter(t *testing.T, data []byte) ([]expectedDiag, []byte) {
	t.Helper()

	// Use goldmark with frontmatter extension to parse metadata.
	md := goldmark.New(goldmark.WithExtensions(&frontmatter.Extender{}))
	ctx := parser.NewContext()
	md.Parser().Parse(text.NewReader(data), parser.WithContext(ctx))

	d := frontmatter.Get(ctx)
	if d == nil {
		return nil, data
	}

	var fm frontMatter
	if err := d.Decode(&fm); err != nil {
		t.Fatalf("decoding front matter: %v", err)
	}

	// Strip the front matter delimiters and YAML from the raw bytes
	// so lint.NewFile sees only markdown content.
	const delim = "---\n"
	rest := data[len(delim):]
	idx := bytes.Index(rest, []byte(delim))
	content := rest[idx+len(delim):]

	return fm.Diagnostics, content
}

func TestRuleFixtures(t *testing.T) {
	dirs, err := filepath.Glob("../../rules/TM*-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no rule fixture directories found")
	}

	for _, dir := range dirs {
		base := filepath.Base(dir)
		m := ruleIDPattern.FindStringSubmatch(base)
		if m == nil {
			t.Errorf("cannot extract rule ID from directory: %s", base)
			continue
		}
		ruleID := m[1]

		t.Run(ruleID, func(t *testing.T) {
			r := rule.ByID(ruleID)
			if r == nil {
				t.Fatalf("rule %s not found in registry", ruleID)
			}

			t.Run("good", func(t *testing.T) {
				src := readFixture(t, filepath.Join(dir, "good.md"))
				f, err := lint.NewFile("good.md", src)
				if err != nil {
					t.Fatalf("parsing good.md: %v", err)
				}
				diags := filterByRule(r.Check(f), ruleID)
				if len(diags) != 0 {
					t.Errorf("good.md: expected 0 diagnostics, got %d", len(diags))
					for _, d := range diags {
						t.Logf("  line %d col %d: %s", d.Line, d.Column, d.Message)
					}
				}
			})

			t.Run("bad", func(t *testing.T) {
				raw := readFixture(t, filepath.Join(dir, "bad.md"))
				expected, src := parseFrontMatter(t, raw)
				f, err := lint.NewFile("bad.md", src)
				if err != nil {
					t.Fatalf("parsing bad.md: %v", err)
				}
				diags := filterByRule(r.Check(f), ruleID)
				if len(expected) == 0 {
					if len(diags) == 0 {
						t.Error("bad.md: expected at least 1 diagnostic, got 0")
					}
				} else {
					if len(diags) != len(expected) {
						t.Errorf("bad.md: expected %d diagnostics, got %d", len(expected), len(diags))
						for _, d := range diags {
							t.Logf("  actual: line %d col %d: %s", d.Line, d.Column, d.Message)
						}
					}
					for i, exp := range expected {
						if i >= len(diags) {
							t.Errorf("missing diagnostic %d: line %d col %d: %s", i, exp.Line, exp.Column, exp.Message)
							continue
						}
						d := diags[i]
						if d.Line != exp.Line || d.Column != exp.Column || d.Message != exp.Message {
							t.Errorf("diagnostic %d:\n  want: line %d col %d: %s\n  got:  line %d col %d: %s",
								i, exp.Line, exp.Column, exp.Message, d.Line, d.Column, d.Message)
						}
					}
				}
			})

			fixedPath := filepath.Join(dir, "fixed.md")
			if _, err := os.Stat(fixedPath); err == nil {
				t.Run("fix", func(t *testing.T) {
					fr, ok := r.(rule.FixableRule)
					if !ok {
						t.Fatalf("fixed.md exists but rule %s does not implement FixableRule", ruleID)
					}

					badSrc := readFixture(t, filepath.Join(dir, "bad.md"))
					_, content := parseFrontMatter(t, badSrc)
					f, err := lint.NewFile("bad.md", content)
					if err != nil {
						t.Fatalf("parsing bad.md: %v", err)
					}

					got := fr.Fix(f)
					want := readFixture(t, fixedPath)

					if !bytes.Equal(got, want) {
						t.Errorf("Fix output does not match fixed.md\ngot:\n%s\nwant:\n%s",
							formatBytes(got), formatBytes(want))
					}
				})
			}
		})
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

func filterByRule(diags []lint.Diagnostic, ruleID string) []lint.Diagnostic {
	var filtered []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == ruleID {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

func formatBytes(b []byte) string {
	s := string(b)
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, " \n", "·\\n")
	return s
}

