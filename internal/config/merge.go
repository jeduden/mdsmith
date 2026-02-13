package config

import (
	"github.com/gobwas/glob"
)

// Merge merges a loaded config on top of defaults. The loaded config's rules
// override the defaults; any rule not mentioned in loaded keeps its default
// value. Ignore and Overrides come from the loaded config only.
// Categories from the loaded config are merged on top of defaults; any
// category not mentioned in loaded keeps its default value (true).
func Merge(defaults, loaded *Config) *Config {
	if loaded == nil {
	// No user config: return a copy of defaults.
	rules := make(map[string]RuleCfg, len(defaults.Rules))
	for k, v := range defaults.Rules {
		rules[k] = v
	}
	cats := copyCategories(defaults.Categories)
	files := copyStrings(defaults.Files)
	noFollow := copyStrings(defaults.NoFollowSymlinks)
	return &Config{
		Rules:            rules,
		FrontMatter:      defaults.FrontMatter,
		Categories:       cats,
		Files:            files,
		NoFollowSymlinks: noFollow,
	}
	}

	rules := make(map[string]RuleCfg, len(defaults.Rules))
	for k, v := range defaults.Rules {
		rules[k] = v
	}

	// Apply loaded rules on top.
	for k, v := range loaded.Rules {
		rules[k] = v
	}

	fm := defaults.FrontMatter
	if loaded.FrontMatter != nil {
		fm = loaded.FrontMatter
	}

	// Merge categories: start with defaults, apply loaded on top.
	cats := mergeCategories(defaults.Categories, loaded.Categories)

	// Merge files: loaded overrides defaults if explicitly set.
	files := copyStrings(defaults.Files)
	filesExplicit := false
	if loaded.FilesExplicit {
		files = copyStrings(loaded.Files)
		filesExplicit = true
	}

	// Track which rules were explicitly set in the loaded config.
	explicit := make(map[string]bool, len(loaded.Rules))
	for k := range loaded.Rules {
		explicit[k] = true
	}

	return &Config{
		Rules:            rules,
		Ignore:           loaded.Ignore,
		Overrides:        loaded.Overrides,
		FrontMatter:      fm,
		Categories:       cats,
		Files:            files,
		FilesExplicit:    filesExplicit,
		ExplicitRules:    explicit,
		NoFollowSymlinks: loaded.NoFollowSymlinks,
	}
}

// copyStrings returns a copy of a string slice. Returns nil if the input is nil.
func copyStrings(s []string) []string {
	if s == nil {
		return nil
	}
	result := make([]string, len(s))
	copy(result, s)
	return result
}

// copyCategories returns a shallow copy of a categories map.
// Returns nil if the input is nil.
func copyCategories(cats map[string]bool) map[string]bool {
	if cats == nil {
		return nil
	}
	result := make(map[string]bool, len(cats))
	for k, v := range cats {
		result[k] = v
	}
	return result
}

// mergeCategories merges override categories on top of base categories.
// If base is nil, a copy of override is returned. If override is nil,
// a copy of base is returned. If both are nil, nil is returned.
func mergeCategories(base, override map[string]bool) map[string]bool {
	if base == nil && override == nil {
		return nil
	}
	result := make(map[string]bool, len(ValidCategories))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

// Effective returns the effective rule configuration for a given file path.
// It starts with the top-level rules and then applies each override whose
// file patterns match filePath, in order. Later overrides take precedence.
func Effective(cfg *Config, filePath string) map[string]RuleCfg {
	result := make(map[string]RuleCfg, len(cfg.Rules))
	for k, v := range cfg.Rules {
		result[k] = v
	}

	for _, o := range cfg.Overrides {
		if matchesAny(o.Files, filePath) {
			for k, v := range o.Rules {
				result[k] = v
			}
		}
	}

	return result
}

// EffectiveExplicitRules returns the set of rule names that were explicitly
// configured for a given file path. It includes rules from the top-level
// ExplicitRules and any rules set by matching overrides.
func EffectiveExplicitRules(cfg *Config, filePath string) map[string]bool {
	result := make(map[string]bool, len(cfg.ExplicitRules))
	for k := range cfg.ExplicitRules {
		result[k] = true
	}

	for _, o := range cfg.Overrides {
		if matchesAny(o.Files, filePath) {
			for k := range o.Rules {
				result[k] = true
			}
		}
	}

	return result
}

// EffectiveCategories returns the effective category settings for a given
// file path. It starts with the top-level categories and then applies each
// override whose file patterns match filePath, in order. Categories not
// explicitly set default to true (enabled).
func EffectiveCategories(cfg *Config, filePath string) map[string]bool {
	// Start with all categories enabled.
	result := make(map[string]bool, len(ValidCategories))
	for _, cat := range ValidCategories {
		result[cat] = true
	}

	// Apply top-level category settings.
	for k, v := range cfg.Categories {
		result[k] = v
	}

	// Apply matching overrides in order.
	for _, o := range cfg.Overrides {
		if matchesAny(o.Files, filePath) {
			for k, v := range o.Categories {
				result[k] = v
			}
		}
	}

	return result
}

// ApplyCategories disables rules whose category is disabled, unless
// the rule has been explicitly configured (present in the explicit rules
// map). ruleCategory maps a rule name to its category string.
// The explicit map contains rule names that were explicitly set in config
// (not just inherited from defaults).
func ApplyCategories(
	rules map[string]RuleCfg,
	categories map[string]bool,
	ruleCategory func(ruleName string) string,
	explicit map[string]bool,
) map[string]RuleCfg {
	result := make(map[string]RuleCfg, len(rules))
	for name, cfg := range rules {
		cat := ruleCategory(name)
		enabled, catSet := categories[cat]
		if catSet && !enabled && !explicit[name] {
			// Category is disabled and rule is not explicitly configured.
			result[name] = RuleCfg{Enabled: false, Settings: cfg.Settings}
		} else {
			result[name] = cfg
		}
	}
	return result
}

// matchesAny returns true if filePath matches any of the given glob patterns.
func matchesAny(patterns []string, filePath string) bool {
	for _, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			// Skip invalid patterns silently.
			continue
		}
		if g.Match(filePath) {
			return true
		}
	}
	return false
}
