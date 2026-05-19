package rules

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/yamlutil"
)

//go:embed MDS*/README.md
var rulesFS embed.FS

// RuleInfo holds metadata extracted from a rule README's front matter.
type RuleInfo struct {
	ID              string
	Name            string
	Status          string
	Description     string
	Content         string
	Maintainability *Maintainability
	Markdownlint    []MarkdownlintRule
}

// MarkdownlintRule names a markdownlint rule that the mdsmith rule covers.
// Partial is true when the mdsmith rule implements only part of the
// corresponding markdownlint check; the coverage matrix at
// docs/research/markdownlint-coverage/README.md is the source of truth.
type MarkdownlintRule struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Partial bool   `yaml:"partial"`
}

// URL returns the canonical markdownlint documentation URL for this rule.
// Markdownlint hosts per-rule docs at a stable per-ID path keyed on the
// lowercase ID, so the URL is derivable from ID alone.
func (r MarkdownlintRule) URL() string {
	return "https://github.com/DavidAnson/markdownlint/blob/main/doc/" +
		strings.ToLower(r.ID) + ".md"
}

// Maintainability captures a rule's adoption pattern: the structural shape a
// reviewer looks for (Signal) and the fix that turns it into the rule's
// declared form (Fix). ForDiagnostic gates whether the fix is appropriate
// to surface on an active diagnostic hover (true) or only as an adoption
// suggestion before the rule fires (false).
type Maintainability struct {
	Signal        string `yaml:"signal"`
	Fix           string `yaml:"fix"`
	ForDiagnostic bool   `yaml:"for-diagnostic"`
}

// ListRules returns all embedded rules sorted by ID.
func ListRules() ([]RuleInfo, error) {
	return listRulesFromFS(rulesFS)
}

// LookupRule finds a rule by ID (e.g. "MDS001") or name (e.g. "line-length")
// and returns its README content with front matter stripped.
func LookupRule(query string) (string, error) {
	return lookupRuleFromFS(rulesFS, query)
}

// LookupRuleInfo finds a rule by ID (e.g. "MDS001") or name (e.g. "line-length")
// and returns its full metadata, including the parsed maintainability block
// and the raw README content (front matter not stripped).
func LookupRuleInfo(query string) (RuleInfo, error) {
	return lookupRuleInfoFromFS(rulesFS, query)
}

func lookupRuleInfoFromFS(fsys fs.FS, query string) (RuleInfo, error) {
	rules, err := listRulesFromFS(fsys)
	if err != nil {
		return RuleInfo{}, err
	}
	q := strings.ToUpper(query)
	for _, r := range rules {
		if strings.ToUpper(r.ID) == q || r.Name == query {
			return r, nil
		}
	}
	return RuleInfo{}, fmt.Errorf("unknown rule %q", query)
}

func listRulesFromFS(fsys fs.FS) ([]RuleInfo, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading rules directory: %w", err)
	}

	var rules []RuleInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := entry.Name() + "/README.md"
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			continue
		}
		info, err := parseFrontMatter(string(data))
		if err != nil {
			continue
		}
		info.Content = string(data)
		rules = append(rules, info)
	}

	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})

	return rules, nil
}

func lookupRuleFromFS(fsys fs.FS, query string) (string, error) {
	rules, err := listRulesFromFS(fsys)
	if err != nil {
		return "", err
	}

	q := strings.ToUpper(query)
	for _, r := range rules {
		if strings.ToUpper(r.ID) == q || r.Name == query {
			return stripFrontMatter(r.Content), nil
		}
	}

	return "", fmt.Errorf("unknown rule %q", query)
}

// parseFrontMatter extracts id, name, status, description, and maintainability
// from YAML front matter. Block scalars (`description: >-`) are folded; any
// embedded newlines collapse to a single space so summaries render on one line.
func parseFrontMatter(content string) (RuleInfo, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return RuleInfo{}, fmt.Errorf("missing front matter")
	}

	var front []string
	terminated := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			terminated = true
			break
		}
		front = append(front, line)
	}
	if err := scanner.Err(); err != nil {
		return RuleInfo{}, fmt.Errorf("scanning front matter: %w", err)
	}
	if !terminated {
		return RuleInfo{}, fmt.Errorf("unterminated front matter")
	}
	var meta struct {
		ID              string             `yaml:"id"`
		Name            string             `yaml:"name"`
		Status          string             `yaml:"status"`
		Description     string             `yaml:"description"`
		Maintainability *Maintainability   `yaml:"maintainability"`
		Markdownlint    []MarkdownlintRule `yaml:"markdownlint"`
	}
	if err := yamlutil.UnmarshalSafe([]byte(strings.Join(front, "\n")), &meta); err != nil {
		return RuleInfo{}, fmt.Errorf("parsing front matter: %w", err)
	}
	info := RuleInfo{
		ID:              meta.ID,
		Name:            meta.Name,
		Status:          meta.Status,
		Description:     collapseWhitespace(meta.Description),
		Maintainability: meta.Maintainability,
		Markdownlint:    meta.Markdownlint,
	}

	if info.ID == "" {
		return RuleInfo{}, fmt.Errorf("front matter missing id")
	}
	if info.Status == "" {
		return RuleInfo{}, fmt.Errorf("front matter missing status")
	}

	return info, nil
}

// collapseWhitespace folds any run of whitespace (including newlines from
// folded block scalars) into a single space so the description renders on
// one line. Leading and trailing whitespace are trimmed.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// StripFrontMatter removes the leading YAML front matter block (--- ... ---)
// and any immediately following blank line from content.
func StripFrontMatter(content string) string {
	return stripFrontMatter(content)
}

// stripFrontMatter removes the leading YAML front matter block (--- ... ---)
// and any immediately following blank line from content.
func stripFrontMatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return content
	}
	body := content[4+end+5:]
	return strings.TrimLeft(body, "\n")
}
