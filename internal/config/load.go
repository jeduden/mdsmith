package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

// maxConfigBytes is a generous cap for the config file size (1 MB).
// Config files should be small; this prevents accidental OOM from
// pointing at a huge file.
const maxConfigBytes int64 = 1024 * 1024

const configFileName = ".mdsmith.yml"

// Load reads and parses a config file at the given path.
func Load(path string) (*Config, error) {
	data, err := readLimitedConfig(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Catch non-string `convention:` values before UnmarshalSafe
	// silently coerces them into the string field.
	if err := validateConventionScalar(data); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	var cfg Config
	if err := yamlutil.UnmarshalSafe(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Detect top-level key presence with a single additional parse so
	// "files" (omitted vs empty) and deprecated keys can be probed
	// without re-parsing per key.
	keys := topLevelKeySet(data)
	cfg.FilesExplicit = keys["files"]

	if keys["no-follow-symlinks"] {
		cfg.Deprecations = append(cfg.Deprecations,
			"config key `no-follow-symlinks` is deprecated; "+
				"symlinks are now skipped by default — "+
				"use `follow-symlinks: true` to opt in, "+
				"or remove the key")
	}

	if keys["archetypes"] {
		cfg.Deprecations = append(cfg.Deprecations,
			"config key `archetypes` has been removed; "+
				"set `required-structure.schema:` to an explicit path, "+
				"or declare a kind under `kinds:` — "+
				"see docs/guides/file-kinds.md")
	}

	detectFilesKeyDeprecations(&cfg)
	detectMetaCategoryDeprecations(&cfg)

	if err := ValidateKinds(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	if err := ValidateBuildConfig(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	if err := applyConvention(&cfg); err != nil {
		return nil, fmt.Errorf("applying convention: %w", err)
	}

	return &cfg, nil
}

// topLevelKeySet returns the set of top-level YAML mapping keys
// present in data, or an empty set on parse error. It rejects
// anchor/alias usage for the same reason yamlHasKey does.
func topLevelKeySet(data []byte) map[string]bool {
	node, err := yamlutil.UnmarshalNodeSafe(data)
	if err != nil {
		return map[string]bool{}
	}
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return map[string]bool{}
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return map[string]bool{}
	}
	result := make(map[string]bool, len(mapping.Content)/2)
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		result[mapping.Content[i].Value] = true
	}
	return result
}

// yamlHasKey returns true if the top-level YAML mapping contains the given key.
func yamlHasKey(data []byte, key string) bool {
	return topLevelKeySet(data)[key]
}

// Discover walks up the directory tree from startDir looking for a
// .mdsmith.yml config file. It stops searching when it encounters a .git
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

// Defaults returns a Config with all registered rules using each rule's
// default enabled state and no custom settings.
func Defaults() *Config {
	all := rule.All()
	rules := make(map[string]RuleCfg, len(all))
	for _, r := range all {
		rules[r.Name()] = RuleCfg{Enabled: enabledByDefault(r)}
	}
	return &Config{
		Rules: rules,
		Files: DefaultFiles,
	}
}

// DumpDefaults returns a Config with all registered rules using each rule's
// default enabled state. Enabled rules that implement Configurable have
// their DefaultSettings() included in RuleCfg.Settings.
// Categories are included with all set to true (enabled).
// This is consumed by `mdsmith init` to generate a default config file.
func DumpDefaults() *Config {
	all := rule.All()
	rules := make(map[string]RuleCfg, len(all))
	for _, r := range all {
		enabled := enabledByDefault(r)
		rc := RuleCfg{Enabled: enabled}
		if enabled {
			if c, ok := r.(rule.Configurable); ok {
				rc.Settings = c.DefaultSettings()
			}
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
		Files:      DefaultFiles,
	}
}

// readLimitedConfig reads a config file with a size cap to prevent OOM.
func readLimitedConfig(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	// Stat for accurate size in error messages.
	var actualSize int64 = -1
	if info, err := f.Stat(); err == nil {
		actualSize = info.Size()
	}

	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxConfigBytes {
		reported := actualSize
		if reported < 0 {
			reported = int64(len(data))
		}
		return nil, fmt.Errorf(
			"config file %q too large (%d bytes, max %d)",
			path, reported, maxConfigBytes,
		)
	}
	return data, nil
}

func detectFilesKeyDeprecations(cfg *Config) {
	for i, o := range cfg.Overrides {
		if o.Files != nil {
			cfg.Deprecations = append(cfg.Deprecations,
				fmt.Sprintf("overrides[%d]: `files:` is deprecated; "+
					"rename it to `glob:` — see docs/reference/globs.md", i))
		}
	}
	for i, e := range cfg.KindAssignment {
		if e.Files != nil {
			cfg.Deprecations = append(cfg.Deprecations,
				fmt.Sprintf("kind-assignment[%d]: `files:` is deprecated; "+
					"rename it to `glob:` — see docs/reference/globs.md", i))
		}
	}
}

// metaNewCategories are the entirely-new categories that replaced the old
// meta bucket. Both were empty before this migration, so setting them does
// not affect any pre-existing rules.
var metaNewCategories = []string{"directive", "structural"}

// metaMovedProseRules lists the rule names that migrated from meta to prose.
// Because prose already contained other rules, these are disabled per-rule
// rather than via categories: {prose: false}, which would also disable rules
// that were never in meta.
var metaMovedProseRules = []string{
	"paragraph-readability",
	"paragraph-structure",
	"token-budget",
	"conciseness-scoring",
	"duplicated-content",
	"emphasis-style",
	"ambiguous-emphasis",
}

// translateMetaCategory rewrites a meta key into its replacement categories
// and returns the meta value (true = enabled, false = disabled). Returns
// (false, false) when meta was not present.
func translateMetaCategory(cats map[string]bool) (v bool, found bool) {
	v, found = cats["meta"]
	if !found {
		return false, false
	}
	for _, cat := range metaNewCategories {
		if _, set := cats[cat]; !set {
			cats[cat] = v
		}
	}
	delete(cats, "meta")
	return v, true
}

// applyMovedProseRules sets per-rule entries for rules that left meta for
// prose. It only writes entries that are not already explicitly configured.
// rules must be non-nil.
func applyMovedProseRules(rules map[string]RuleCfg, enabled bool) {
	for _, name := range metaMovedProseRules {
		if _, set := rules[name]; !set {
			rules[name] = RuleCfg{Enabled: enabled}
		}
	}
}

func detectMetaCategoryDeprecations(cfg *Config) {
	const msg = "category `meta` has been split into `directive`, `structural`, and `prose`; " +
		"update your `categories:` block to use the new names, and disable moved prose " +
		"rules (paragraph-readability, paragraph-structure, token-budget, " +
		"conciseness-scoring, duplicated-content, emphasis-style, ambiguous-emphasis) " +
		"by rule name if needed"
	warned := false
	if v, ok := translateMetaCategory(cfg.Categories); ok {
		// Only disable prose rules explicitly; enabling them (meta: true) must
		// not opt in default-disabled rules that meta: true never activated.
		if !v {
			if cfg.Rules == nil {
				cfg.Rules = make(map[string]RuleCfg)
			}
			applyMovedProseRules(cfg.Rules, false)
		}
		cfg.Deprecations = append(cfg.Deprecations, msg)
		warned = true
	}
	for name, kind := range cfg.Kinds {
		if v, ok := translateMetaCategory(kind.Categories); ok {
			if !v {
				if kind.Rules == nil {
					kind.Rules = make(map[string]RuleCfg)
				}
				applyMovedProseRules(kind.Rules, false)
			}
			cfg.Kinds[name] = kind
			if !warned {
				cfg.Deprecations = append(cfg.Deprecations, msg)
				warned = true
			}
		}
	}
	for i, o := range cfg.Overrides {
		if v, ok := translateMetaCategory(o.Categories); ok {
			if !v {
				if o.Rules == nil {
					o.Rules = make(map[string]RuleCfg)
				}
				applyMovedProseRules(o.Rules, false)
			}
			cfg.Overrides[i] = o
			if !warned {
				cfg.Deprecations = append(cfg.Deprecations, msg)
				warned = true
			}
		}
	}
}

func enabledByDefault(r rule.Rule) bool {
	if d, ok := r.(rule.Defaultable); ok {
		return d.EnabledByDefault()
	}
	return true
}
