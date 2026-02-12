package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeduden/tidymark/internal/rule"
	"gopkg.in/yaml.v3"
)

const configFileName = ".tidymark.yml"

// allRuleNames lists the 22 built-in rule identifiers in order.
var allRuleNames = []string{
	"line-length",
	"heading-style",
	"heading-increment",
	"first-line-heading",
	"no-duplicate-headings",
	"no-trailing-spaces",
	"no-hard-tabs",
	"no-multiple-blanks",
	"single-trailing-newline",
	"fenced-code-style",
	"fenced-code-language",
	"no-bare-urls",
	"blank-line-around-headings",
	"blank-line-around-lists",
	"blank-line-around-fenced-code",
	"list-indent",
	"no-trailing-punctuation-in-heading",
	"no-emphasis-as-heading",
	"catalog",
	"required-structure",
	"include",
	"table-format",
}

// Load reads and parses a config file at the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}

// Discover walks up the directory tree from startDir looking for a
// .tidymark.yml config file. It stops searching when it encounters a .git
// directory (the repository root) or reaches the filesystem root.
// Returns the path to the config file, or "" if none was found.
func Discover(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}

	for {
		candidate := filepath.Join(dir, configFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		// Check for .git boundary — if .git exists in this dir,
		// this is the repo root and we should not search further up.
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return "", nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", nil
		}
		dir = parent
	}
}

// Defaults returns a Config with all 19 built-in rules enabled
// with default settings (no custom settings).
func Defaults() *Config {
	rules := make(map[string]RuleCfg, len(allRuleNames))
	for _, name := range allRuleNames {
		rules[name] = RuleCfg{Enabled: true}
	}
	return &Config{
		Rules: rules,
	}
}

// DumpDefaults returns a Config with all registered rules enabled and
// their default settings populated. Rules that implement Configurable
// have their DefaultSettings() included in RuleCfg.Settings.
// Categories are included with all set to true (enabled).
// This is consumed by `tidymark init` to generate a default config file.
func DumpDefaults() *Config {
	all := rule.All()
	rules := make(map[string]RuleCfg, len(all))
	for _, r := range all {
		rc := RuleCfg{Enabled: true}
		if c, ok := r.(rule.Configurable); ok {
			rc.Settings = c.DefaultSettings()
		}
		rules[r.Name()] = rc
	}

	categories := make(map[string]bool, len(ValidCategories))
	for _, cat := range ValidCategories {
		categories[cat] = true
	}

	return &Config{
		Rules:      rules,
		Categories: categories,
	}
}
