package rules

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

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
	Category        string
	Content         string
	Maintainability *Maintainability
	Markdownlint    []RuleMapping
	Rumdl           []RuleMapping
	Mado            []RuleMapping
	Panache         []RuleMapping
	ObsidianLinter  []RuleMapping
}

// RuleMapping names a rule in a peer Markdown linter that the mdsmith rule
// covers, plus that linter's default-on/off state for the rule. Partial is
// true when the mdsmith rule implements only part of the peer check.
//
// Go's YAML decoder does not enforce the proto schema; a missing
// `default:` key decodes to the bool zero value (false). The proto
// schema (validated by MDS020 in `mdsmith check`) makes `default:`
// required, so the repo's CI gate catches a missing key before it
// reaches a downstream generator. Tools that walk the embedded rule
// metadata without running `mdsmith check` first should treat
// `Default == false` cautiously when reviewing a freshly authored
// rule README.
//
// The per-rule front-matter blocks are the source of truth;
// docs/research/markdownlint-coverage/README.md is regenerated from
// them by `mdsmith-release sync-coverage-matrix`.
type RuleMapping struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Partial bool   `yaml:"partial"`
	Default bool   `yaml:"default"`
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

// docSiteBase is the published rules-section base URL. SyncDocs serves
// internal/rules/<ID>-<name>/README.md at <docSiteBase><lower(ID-name)>/,
// so a rule's doc-page slug is its directory name lowercased (Hugo
// case-folds path-derived URLs).
const docSiteBase = "https://mdsmith.dev/rules/"

var docURLOnce struct {
	sync.Once
	m map[string]string
}

// DocURL returns the canonical website documentation URL for a rule ID
// (e.g. "MDS020" → "https://mdsmith.dev/rules/mds020-required-structure/"),
// or "" when the ID is unknown. The map is built once from the embedded
// rule list and cached; safe for concurrent use.
func DocURL(id string) string {
	docURLOnce.Do(func() {
		all, _ := ListRules()
		m := make(map[string]string, len(all))
		for _, r := range all {
			slug := strings.ToLower(r.ID + "-" + r.Name)
			m[strings.ToUpper(r.ID)] = docSiteBase + slug + "/"
		}
		docURLOnce.m = m
	})
	return docURLOnce.m[strings.ToUpper(id)]
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
	for _, r := range rules {
		if strings.EqualFold(r.ID, query) || r.Name == query {
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

	for _, r := range rules {
		if strings.EqualFold(r.ID, query) || r.Name == query {
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
		ID              string           `yaml:"id"`
		Name            string           `yaml:"name"`
		Status          string           `yaml:"status"`
		Description     string           `yaml:"description"`
		Category        string           `yaml:"category"`
		Maintainability *Maintainability `yaml:"maintainability"`
		Markdownlint    []RuleMapping    `yaml:"markdownlint"`
		Rumdl           []RuleMapping    `yaml:"rumdl"`
		Mado            []RuleMapping    `yaml:"mado"`
		Panache         []RuleMapping    `yaml:"panache"`
		ObsidianLinter  []RuleMapping    `yaml:"obsidian-linter"`
	}
	if err := yamlutil.UnmarshalSafe([]byte(strings.Join(front, "\n")), &meta); err != nil {
		return RuleInfo{}, fmt.Errorf("parsing front matter: %w", err)
	}
	info := RuleInfo{
		ID:              meta.ID,
		Name:            meta.Name,
		Status:          meta.Status,
		Description:     collapseWhitespace(meta.Description),
		Category:        meta.Category,
		Maintainability: meta.Maintainability,
		Markdownlint:    meta.Markdownlint,
		Rumdl:           meta.Rumdl,
		Mado:            meta.Mado,
		Panache:         meta.Panache,
		ObsidianLinter:  meta.ObsidianLinter,
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
