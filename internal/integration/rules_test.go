package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/frontmatter"

	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundlists"
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/crossfilereferenceintegrity"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/mdsmith/internal/rules/firstlineheading"
	_ "github.com/jeduden/mdsmith/internal/rules/headingincrement"
	_ "github.com/jeduden/mdsmith/internal/rules/headingstyle"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/listindent"
	_ "github.com/jeduden/mdsmith/internal/rules/maxfilelength"
	_ "github.com/jeduden/mdsmith/internal/rules/nobareurls"
	_ "github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/mdsmith/internal/rules/nohardtabs"
	_ "github.com/jeduden/mdsmith/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphreadability"
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphstructure"
	_ "github.com/jeduden/mdsmith/internal/rules/requiredstructure"
	_ "github.com/jeduden/mdsmith/internal/rules/singletrailingnewline"
	_ "github.com/jeduden/mdsmith/internal/rules/tableformat"
	_ "github.com/jeduden/mdsmith/internal/rules/tablereadability"
	_ "github.com/jeduden/mdsmith/internal/rules/tokenbudget"
)

var ruleIDPattern = regexp.MustCompile(`^(MDS\d+)-`)

type expectedDiag struct {
	Line    int    `yaml:"line"`
	Column  int    `yaml:"column"`
	Message string `yaml:"message"`
}

type fixtureFrontMatter struct {
	Settings    map[string]any `yaml:"settings"`
	Diagnostics []expectedDiag `yaml:"diagnostics"`
}

// parseFixtureFrontMatter extracts YAML front matter from markdown using
// goldmark-frontmatter, then strips it from the raw bytes so lint.NewFile
// receives plain markdown content. Returns settings, diagnostics, and content.
func parseFixtureFrontMatter(
	t *testing.T, data []byte,
) (map[string]any, []expectedDiag, []byte) {
	t.Helper()

	md := goldmark.New(goldmark.WithExtensions(&frontmatter.Extender{}))
	ctx := parser.NewContext()
	md.Parser().Parse(text.NewReader(data), parser.WithContext(ctx))

	d := frontmatter.Get(ctx)
	if d == nil {
		return nil, nil, data
	}

	var fm fixtureFrontMatter
	if err := d.Decode(&fm); err != nil {
		t.Fatalf("decoding front matter: %v", err)
	}

	// Strip the front matter delimiters and YAML from the raw bytes
	// so lint.NewFile sees only markdown content.
	const delim = "---\n"
	rest := data[len(delim):]
	idx := bytes.Index(rest, []byte(delim))
	content := rest[idx+len(delim):]

	return fm.Settings, fm.Diagnostics, content
}

// applySettingsToRule applies fixture settings to a rule. It saves and restores
// the default settings after the test to avoid polluting the global singleton.
func applySettingsToRule(
	t *testing.T, r rule.Rule, settings map[string]any,
) {
	t.Helper()
	if len(settings) == 0 {
		return
	}

	cr, ok := r.(rule.Configurable)
	if !ok {
		t.Fatalf(
			"fixture specifies settings but rule %s does not implement Configurable",
			r.ID(),
		)
	}

	defaults := cr.DefaultSettings()
	t.Cleanup(func() {
		_ = cr.ApplySettings(defaults)
	})

	if err := cr.ApplySettings(settings); err != nil {
		t.Fatalf("applying settings: %v", err)
	}
}

func TestRuleFixtures(t *testing.T) {
	dirs := discoverFixtureDirs(t)

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

			// Determine format: folder-based or single-file.
			badDir := filepath.Join(dir, "bad")
			goodDir := filepath.Join(dir, "good")
			hasBadDir := isDir(badDir)
			hasGoodDir := isDir(goodDir)
			useFolderFormat := hasBadDir || hasGoodDir

			if useFolderFormat {
				runFolderFixtures(t, dir, r, ruleID)
			} else {
				runSingleFileFixtures(t, dir, r, ruleID)
			}
		})
	}
}

// runSingleFileFixtures runs the old single-file format (bad.md, good.md,
// fixed.md). Kept for backward compatibility.
func runSingleFileFixtures(
	t *testing.T, dir string, r rule.Rule, ruleID string,
) {
	t.Helper()

	t.Run("good", func(t *testing.T) {
		runGoodSingleFile(t, dir)
	})
	t.Run("bad", func(t *testing.T) {
		runBadSingleFile(t, dir, r, ruleID)
	})

	fixedPath := filepath.Join(dir, "fixed.md")
	if _, err := os.Stat(fixedPath); err == nil {
		t.Run("fix", func(t *testing.T) {
			runFixSingleFile(t, dir, r, ruleID)
		})
	}
}

// runFolderFixtures runs the new folder-based format with bad/, good/,
// and fixed/ subdirectories.
func runFolderFixtures(
	t *testing.T, dir string, r rule.Rule, ruleID string,
) {
	t.Helper()

	goodDir := filepath.Join(dir, "good")
	if isDir(goodDir) {
		t.Run("good", func(t *testing.T) {
			files := discoverMDFiles(t, goodDir)
			for _, f := range files {
				name := strings.TrimSuffix(filepath.Base(f), ".md")
				t.Run(name, func(t *testing.T) {
					runGoodFolderFile(t, dir, f, r)
				})
			}
		})
	}

	badDir := filepath.Join(dir, "bad")
	if isDir(badDir) {
		t.Run("bad", func(t *testing.T) {
			files := discoverMDFiles(t, badDir)
			for _, f := range files {
				name := strings.TrimSuffix(filepath.Base(f), ".md")
				t.Run(name, func(t *testing.T) {
					runBadFolderFile(t, dir, f, r, ruleID)
				})
			}
		})
	}

	fixedDir := filepath.Join(dir, "fixed")
	if isDir(fixedDir) {
		t.Run("fix", func(t *testing.T) {
			files := discoverMDFiles(t, fixedDir)
			for _, f := range files {
				name := strings.TrimSuffix(filepath.Base(f), ".md")
				t.Run(name, func(t *testing.T) {
					runFixFolderFile(t, dir, f, r, ruleID)
				})
			}
		})
	}
}

// runGoodFolderFile checks a single good fixture file from a folder.
func runGoodFolderFile(
	t *testing.T, ruleDir string, filePath string, r rule.Rule,
) {
	t.Helper()
	raw := readFixture(t, filePath)
	settings, _, content := parseFixtureFrontMatter(t, raw)
	applySettingsToRule(t, r, settings)

	f, err := lint.NewFile(filepath.Base(filePath), content)
	if err != nil {
		t.Fatalf("parsing %s: %v", filepath.Base(filePath), err)
	}
	f.FS = os.DirFS(filepath.Dir(filePath))
	diags := checkAllRules(f)
	reportUnexpectedDiags(t, filepath.Base(filePath), diags)
}

// runBadFolderFile checks a single bad fixture file from a folder.
func runBadFolderFile(
	t *testing.T, ruleDir string, filePath string,
	r rule.Rule, ruleID string,
) {
	t.Helper()
	raw := readFixture(t, filePath)
	settings, expected, content := parseFixtureFrontMatter(t, raw)
	applySettingsToRule(t, r, settings)

	f, err := lint.NewFile(filepath.Base(filePath), content)
	if err != nil {
		t.Fatalf("parsing %s: %v", filepath.Base(filePath), err)
	}
	f.FS = os.DirFS(filepath.Dir(filePath))
	diags := filterByRule(r.Check(f), ruleID)
	assertExpectedDiags(t, expected, diags, filepath.Base(filePath))
}

// runFixFolderFile loads the matching bad/ file, applies settings, runs Fix,
// and compares output against the fixed/ file body.
func runFixFolderFile(
	t *testing.T, ruleDir string, fixedPath string,
	r rule.Rule, ruleID string,
) {
	t.Helper()
	fr, ok := r.(rule.FixableRule)
	if !ok {
		t.Fatalf(
			"fixed/ exists but rule %s does not implement FixableRule",
			ruleID,
		)
	}

	// Load matching bad/ file.
	badPath := filepath.Join(
		ruleDir, "bad", filepath.Base(fixedPath),
	)
	badRaw := readFixture(t, badPath)
	settings, _, badContent := parseFixtureFrontMatter(t, badRaw)
	applySettingsToRule(t, r, settings)

	f, err := lint.NewFile(filepath.Base(fixedPath), badContent)
	if err != nil {
		t.Fatalf("parsing %s: %v", filepath.Base(fixedPath), err)
	}
	f.FS = os.DirFS(filepath.Dir(fixedPath))

	got := fr.Fix(f)

	// Load fixed/ file and strip its frontmatter.
	fixedRaw := readFixture(t, fixedPath)
	_, _, want := parseFixtureFrontMatter(t, fixedRaw)

	if !bytes.Equal(got, want) {
		t.Errorf(
			"Fix output does not match %s\ngot:\n%s\nwant:\n%s",
			filepath.Base(fixedPath),
			formatBytes(got), formatBytes(want),
		)
	}

	// Verify that the fixed output produces no diagnostics.
	fixedFile, err := lint.NewFile(
		filepath.Base(fixedPath), want,
	)
	if err != nil {
		t.Fatalf("parsing fixed output: %v", err)
	}
	fixedFile.FS = os.DirFS(filepath.Dir(fixedPath))
	diags := checkAllRules(fixedFile)
	reportUnexpectedDiags(t, filepath.Base(fixedPath), diags)
}

// --- single-file format helpers (backward compat) ---

func runGoodSingleFile(t *testing.T, dir string) {
	t.Helper()
	src := readFixture(t, filepath.Join(dir, "good.md"))
	f, err := lint.NewFile("good.md", src)
	if err != nil {
		t.Fatalf("parsing good.md: %v", err)
	}
	f.FS = os.DirFS(dir)
	diags := checkAllRules(f)
	reportUnexpectedDiags(t, "good.md", diags)
}

func runBadSingleFile(
	t *testing.T, dir string, r rule.Rule, ruleID string,
) {
	t.Helper()
	raw := readFixture(t, filepath.Join(dir, "bad.md"))
	_, expected, src := parseFixtureFrontMatter(t, raw)
	f, err := lint.NewFile("bad.md", src)
	if err != nil {
		t.Fatalf("parsing bad.md: %v", err)
	}
	f.FS = os.DirFS(dir)
	diags := filterByRule(r.Check(f), ruleID)
	assertExpectedDiags(t, expected, diags, "bad.md")
}

func runFixSingleFile(
	t *testing.T, dir string, r rule.Rule, ruleID string,
) {
	t.Helper()
	fr, ok := r.(rule.FixableRule)
	if !ok {
		t.Fatalf(
			"fixed.md exists but rule %s does not implement FixableRule",
			ruleID,
		)
	}

	badSrc := readFixture(t, filepath.Join(dir, "bad.md"))
	_, _, content := parseFixtureFrontMatter(t, badSrc)
	f, err := lint.NewFile("bad.md", content)
	if err != nil {
		t.Fatalf("parsing bad.md: %v", err)
	}
	f.FS = os.DirFS(dir)

	got := fr.Fix(f)
	fixedPath := filepath.Join(dir, "fixed.md")
	want := readFixture(t, fixedPath)

	if !bytes.Equal(got, want) {
		t.Errorf(
			"Fix output does not match fixed.md\ngot:\n%s\nwant:\n%s",
			formatBytes(got), formatBytes(want),
		)
	}

	fixedFile, err := lint.NewFile("fixed.md", want)
	if err != nil {
		t.Fatalf("parsing fixed.md: %v", err)
	}
	fixedFile.FS = os.DirFS(dir)
	diags := checkAllRules(fixedFile)
	reportUnexpectedDiags(t, "fixed.md", diags)
}

// --- shared helpers ---

func discoverFixtureDirs(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob("../../rules/MDS*-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no rule fixture directories found")
	}
	return dirs
}

func discoverMDFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no .md files found in %s", dir)
	}
	return files
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func reportUnexpectedDiags(
	t *testing.T, filename string, diags []lint.Diagnostic,
) {
	t.Helper()
	if len(diags) != 0 {
		t.Errorf(
			"%s: expected 0 diagnostics from all rules, got %d",
			filename, len(diags),
		)
		for _, d := range diags {
			t.Logf(
				"  %s line %d col %d: %s",
				d.RuleID, d.Line, d.Column, d.Message,
			)
		}
	}
}

func assertExpectedDiags(
	t *testing.T,
	expected []expectedDiag,
	diags []lint.Diagnostic,
	filename string,
) {
	t.Helper()
	if len(expected) == 0 {
		if len(diags) == 0 {
			t.Errorf(
				"%s: expected at least 1 diagnostic, got 0",
				filename,
			)
		}
		return
	}
	if len(diags) != len(expected) {
		t.Errorf(
			"%s: expected %d diagnostics, got %d",
			filename, len(expected), len(diags),
		)
		for _, d := range diags {
			t.Logf(
				"  actual: line %d col %d: %s",
				d.Line, d.Column, d.Message,
			)
		}
	}
	for i, exp := range expected {
		if i >= len(diags) {
			t.Errorf(
				"missing diagnostic %d: line %d col %d: %s",
				i, exp.Line, exp.Column, exp.Message,
			)
			continue
		}
		d := diags[i]
		if d.Line != exp.Line || d.Column != exp.Column || d.Message != exp.Message {
			t.Errorf(
				"diagnostic %d:\n  want: line %d col %d: %s\n  got:  line %d col %d: %s",
				i, exp.Line, exp.Column, exp.Message,
				d.Line, d.Column, d.Message,
			)
		}
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

func checkAllRules(f *lint.File) []lint.Diagnostic {
	var all []lint.Diagnostic
	for _, r := range rule.All() {
		all = append(all, r.Check(f)...)
	}
	return all
}

func filterByRule(
	diags []lint.Diagnostic, ruleID string,
) []lint.Diagnostic {
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
	s = strings.ReplaceAll(s, " \n", "\u00b7\\n")
	return s
}
