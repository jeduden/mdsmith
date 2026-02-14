package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	// Import all rule packages so their init() functions register rules.
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
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/listindent"
	_ "github.com/jeduden/mdsmith/internal/rules/nobareurls"
	_ "github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/mdsmith/internal/rules/nohardtabs"
	_ "github.com/jeduden/mdsmith/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
	_ "github.com/jeduden/mdsmith/internal/rules/singletrailingnewline"
	_ "github.com/jeduden/mdsmith/internal/rules/tokenbudget"
)

// categorizedMockRule is a mock rule with a configurable category.
type categorizedMockRule struct {
	id       string
	name     string
	category string
}

func (r *categorizedMockRule) ID() string       { return r.id }
func (r *categorizedMockRule) Name() string     { return r.name }
func (r *categorizedMockRule) Category() string { return r.category }
func (r *categorizedMockRule) Check(f *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  r.name + " violation",
		},
	}
}

func TestCategories_DisablingCategoryDisablesAllItsRules(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")

	// Content that would trigger heading rules:
	// - heading-increment (MDS003): skips from h1 to h3
	// - first-line-heading (MDS004): first line is not a heading
	// But we also need the file to be valid for non-heading rules so
	// we only get heading diagnostics.
	content := "Some text first.\n\n# Heading 1\n\n### Heading 3\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Enable all rules but disable the heading category.
	allRules := rule.All()
	rulesCfg := make(map[string]config.RuleCfg, len(allRules))
	for _, r := range allRules {
		rulesCfg[r.Name()] = config.RuleCfg{Enabled: true}
	}

	cfg := &config.Config{
		Rules:      rulesCfg,
		Categories: map[string]bool{"heading": false},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  allRules,
	}

	result := runner.Run([]string{mdFile})

	// Verify no diagnostics are from heading rules.
	headingRules := rulesByCategory(allRules, "heading")
	for _, d := range result.Diagnostics {
		if headingRules[d.RuleName] {
			t.Errorf("heading category disabled but got diagnostic from heading rule %s (%s): %s",
				d.RuleName, d.RuleID, d.Message)
		}
	}
}

func TestCategories_DisabledCategoryWithExplicitRuleOverride(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")

	// Content that triggers heading-increment (skips from h1 to h3)
	// and first-line-heading (first line is not a heading).
	content := "Some text first.\n\n# Heading 1\n\n### Heading 3\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	allRules := rule.All()
	rulesCfg := make(map[string]config.RuleCfg, len(allRules))
	for _, r := range allRules {
		rulesCfg[r.Name()] = config.RuleCfg{Enabled: true}
	}

	cfg := &config.Config{
		Rules:      rulesCfg,
		Categories: map[string]bool{"heading": false},
		// Explicitly enable heading-increment to override the disabled category.
		ExplicitRules: map[string]bool{"heading-increment": true},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  allRules,
	}

	result := runner.Run([]string{mdFile})

	// heading-increment should produce diagnostics (explicit override).
	foundHeadingIncrement := false
	for _, d := range result.Diagnostics {
		if d.RuleName == "heading-increment" {
			foundHeadingIncrement = true
		}
	}
	if !foundHeadingIncrement {
		t.Error("expected heading-increment diagnostics (explicit override) but found none")
	}

	// Other heading rules (e.g. first-line-heading) should be disabled.
	headingRules := rulesByCategory(allRules, "heading")
	for _, d := range result.Diagnostics {
		if headingRules[d.RuleName] && d.RuleName != "heading-increment" {
			t.Errorf("heading category disabled but got diagnostic from non-explicit heading rule %s (%s): %s",
				d.RuleName, d.RuleID, d.Message)
		}
	}
}

func TestCategories_AllCategoriesEnabledByDefault(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")

	// Content that triggers rules from multiple categories:
	// - heading category: first-line-heading (first line is not a heading)
	// - whitespace category: no-trailing-spaces (trailing spaces)
	content := "Some text with trailing spaces.   \n\n# Heading\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	allRules := rule.All()
	rulesCfg := make(map[string]config.RuleCfg, len(allRules))
	for _, r := range allRules {
		rulesCfg[r.Name()] = config.RuleCfg{Enabled: true}
	}

	// No categories set -- all should be enabled by default.
	cfg := &config.Config{
		Rules: rulesCfg,
	}

	runner := &Runner{
		Config: cfg,
		Rules:  allRules,
	}

	result := runner.Run([]string{mdFile})

	// Verify we get diagnostics from at least two different categories.
	categories := make(map[string]bool)
	catLookup := make(map[string]string)
	for _, r := range allRules {
		catLookup[r.Name()] = r.Category()
	}
	for _, d := range result.Diagnostics {
		if cat, ok := catLookup[d.RuleName]; ok {
			categories[cat] = true
		}
	}

	if len(categories) < 2 {
		t.Errorf("expected diagnostics from at least 2 categories, got %d: %v",
			len(categories), categories)
		for _, d := range result.Diagnostics {
			t.Logf("  %s (%s): %s", d.RuleName, d.RuleID, d.Message)
		}
	}
}

func TestCategories_OverrideDisablesCategoryForMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	txtFile := filepath.Join(dir, "test.txt")

	// Content with trailing spaces (triggers no-trailing-spaces, a whitespace rule).
	content := "# Heading\n\nSome text with trailing spaces.   \n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txtFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	allRules := rule.All()
	rulesCfg := make(map[string]config.RuleCfg, len(allRules))
	for _, r := range allRules {
		rulesCfg[r.Name()] = config.RuleCfg{Enabled: true}
	}

	cfg := &config.Config{
		Rules: rulesCfg,
		Overrides: []config.Override{
			{
				Files:      []string{"*.md"},
				Categories: map[string]bool{"whitespace": false},
			},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  allRules,
	}

	// Check the .md file -- whitespace rules should be disabled.
	resultMD := runner.Run([]string{mdFile})
	whitespaceRules := rulesByCategory(allRules, "whitespace")
	for _, d := range resultMD.Diagnostics {
		if whitespaceRules[d.RuleName] {
			t.Errorf("whitespace category disabled for *.md but got diagnostic from whitespace rule %s (%s): %s",
				d.RuleName, d.RuleID, d.Message)
		}
	}

	// Check the .txt file -- whitespace rules should still be active.
	resultTXT := runner.Run([]string{txtFile})
	foundWhitespace := false
	for _, d := range resultTXT.Diagnostics {
		if whitespaceRules[d.RuleName] {
			foundWhitespace = true
			break
		}
	}
	if !foundWhitespace {
		t.Error("expected whitespace diagnostics for .txt file (no override), but found none")
	}
}

func TestCategories_DisablingCategoryWithMockRules(t *testing.T) {
	// Use mock rules with known categories for precise control.
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	headingRule := &categorizedMockRule{id: "MDS900", name: "mock-heading", category: "heading"}
	whitespaceRule := &categorizedMockRule{id: "MDS901", name: "mock-whitespace", category: "whitespace"}
	codeRule := &categorizedMockRule{id: "MDS902", name: "mock-code", category: "code"}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-heading":    {Enabled: true},
			"mock-whitespace": {Enabled: true},
			"mock-code":       {Enabled: true},
		},
		Categories: map[string]bool{"heading": false},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{headingRule, whitespaceRule, codeRule},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// Should have diagnostics from whitespace and code, but NOT heading.
	ruleIDs := make(map[string]bool)
	for _, d := range result.Diagnostics {
		ruleIDs[d.RuleID] = true
	}

	if ruleIDs["MDS900"] {
		t.Error("heading category disabled but got diagnostic from mock-heading (MDS900)")
	}
	if !ruleIDs["MDS901"] {
		t.Error("expected diagnostic from mock-whitespace (MDS901)")
	}
	if !ruleIDs["MDS902"] {
		t.Error("expected diagnostic from mock-code (MDS902)")
	}
}

func TestCategories_ExplicitRuleOverrideWithMockRules(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	headingRule1 := &categorizedMockRule{id: "MDS900", name: "mock-heading-a", category: "heading"}
	headingRule2 := &categorizedMockRule{id: "MDS901", name: "mock-heading-b", category: "heading"}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-heading-a": {Enabled: true},
			"mock-heading-b": {Enabled: true},
		},
		Categories: map[string]bool{"heading": false},
		// Only mock-heading-a is explicitly set.
		ExplicitRules: map[string]bool{"mock-heading-a": true},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{headingRule1, headingRule2},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	ruleIDs := make(map[string]bool)
	for _, d := range result.Diagnostics {
		ruleIDs[d.RuleID] = true
	}

	// mock-heading-a should fire (explicit override).
	if !ruleIDs["MDS900"] {
		t.Error("expected diagnostic from mock-heading-a (MDS900) due to explicit override")
	}
	// mock-heading-b should NOT fire (category disabled, not explicit).
	if ruleIDs["MDS901"] {
		t.Error("did not expect diagnostic from mock-heading-b (MDS901), heading category is disabled")
	}
}

func TestCategories_RunSourceRespectsCategories(t *testing.T) {
	headingRule := &categorizedMockRule{id: "MDS900", name: "mock-heading", category: "heading"}
	whitespaceRule := &categorizedMockRule{id: "MDS901", name: "mock-whitespace", category: "whitespace"}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-heading":    {Enabled: true},
			"mock-whitespace": {Enabled: true},
		},
		Categories: map[string]bool{"heading": false},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{headingRule, whitespaceRule},
	}

	result := runner.RunSource("test.md", []byte("# Hello\n"))
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	ruleIDs := make(map[string]bool)
	for _, d := range result.Diagnostics {
		ruleIDs[d.RuleID] = true
	}

	if ruleIDs["MDS900"] {
		t.Error("heading category disabled but got diagnostic from mock-heading via RunSource")
	}
	if !ruleIDs["MDS901"] {
		t.Error("expected diagnostic from mock-whitespace via RunSource")
	}
}

func TestCategories_OverrideCategoryWithMockRules(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "docs.md")
	txtFile := filepath.Join(dir, "docs.txt")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txtFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	codeRule := &categorizedMockRule{id: "MDS900", name: "mock-code", category: "code"}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-code": {Enabled: true},
		},
		Overrides: []config.Override{
			{
				Files:      []string{"*.md"},
				Categories: map[string]bool{"code": false},
			},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{codeRule},
	}

	// .md file: code category disabled via override.
	resultMD := runner.Run([]string{mdFile})
	if len(resultMD.Diagnostics) != 0 {
		t.Errorf("code category disabled for *.md but got %d diagnostics", len(resultMD.Diagnostics))
	}

	// .txt file: code category still active (override does not match).
	resultTXT := runner.Run([]string{txtFile})
	if len(resultTXT.Diagnostics) != 1 {
		t.Errorf("expected 1 diagnostic for .txt file, got %d", len(resultTXT.Diagnostics))
	}
}

// TestAllRuleCategoriesValid verifies that every registered rule returns
// a valid category name from the known set.
func TestAllRuleCategoriesValid(t *testing.T) {
	validSet := make(map[string]bool, len(config.ValidCategories))
	for _, cat := range config.ValidCategories {
		validSet[cat] = true
	}

	allRules := rule.All()
	if len(allRules) == 0 {
		t.Fatal("no rules registered; blank imports may be missing")
	}

	for _, r := range allRules {
		cat := r.Category()
		if !validSet[cat] {
			t.Errorf("rule %s (%s) has invalid category %q; valid categories: %v",
				r.Name(), r.ID(), cat, config.ValidCategories)
		}
	}
}

// TestAllRulesHaveNonEmptyCategory verifies that no rule returns an empty
// category string.
func TestAllRulesHaveNonEmptyCategory(t *testing.T) {
	for _, r := range rule.All() {
		if r.Category() == "" {
			t.Errorf("rule %s (%s) returns empty category", r.Name(), r.ID())
		}
	}
}

// rulesByCategory returns a set of rule names belonging to the given category.
func rulesByCategory(rules []rule.Rule, category string) map[string]bool {
	result := make(map[string]bool)
	for _, r := range rules {
		if r.Category() == category {
			result[r.Name()] = true
		}
	}
	return result
}
