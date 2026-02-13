package tidymark

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed rules/*/README.md
var rulesFS embed.FS

// RuleInfo holds metadata extracted from a rule README's front matter.
type RuleInfo struct {
	ID          string
	Name        string
	Description string
	Content     string
}

// ListRules returns all embedded rules sorted by ID.
func ListRules() ([]RuleInfo, error) {
	return listRulesFromFS(rulesFS)
}

// LookupRule finds a rule by ID (e.g. "TM001") or name (e.g. "line-length")
// and returns its full README content.
func LookupRule(query string) (string, error) {
	return lookupRuleFromFS(rulesFS, query)
}

func listRulesFromFS(fsys fs.FS) ([]RuleInfo, error) {
	entries, err := fs.ReadDir(fsys, "rules")
	if err != nil {
		return nil, fmt.Errorf("reading rules directory: %w", err)
	}

	var rules []RuleInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := "rules/" + entry.Name() + "/README.md"
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
			return r.Content, nil
		}
	}

	return "", fmt.Errorf("unknown rule %q", query)
}

// parseFrontMatter extracts id, name, and description from YAML front matter.
func parseFrontMatter(content string) (RuleInfo, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return RuleInfo{}, fmt.Errorf("missing front matter")
	}

	var info RuleInfo
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, val, ok := parseYAMLLine(line)
		if !ok {
			continue
		}
		switch key {
		case "id":
			info.ID = val
		case "name":
			info.Name = val
		case "description":
			info.Description = val
		}
	}

	if info.ID == "" {
		return RuleInfo{}, fmt.Errorf("front matter missing id")
	}

	return info, nil
}

// parseYAMLLine parses a simple "key: value" line.
func parseYAMLLine(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	return key, value, true
}
