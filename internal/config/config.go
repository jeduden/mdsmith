package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ValidCategories lists the recognized rule category names.
var ValidCategories = []string{
	"heading",
	"whitespace",
	"code",
	"list",
	"line",
	"link",
	"meta",
	"table",
}

// DefaultFiles is the built-in list of glob patterns used for file
// discovery when no file arguments are given on the command line.
var DefaultFiles = []string{"**/*.md", "**/*.markdown"}

// Config is the top-level configuration.
type Config struct {
	Rules          map[string]RuleCfg    `yaml:"rules"`
	Ignore         []string              `yaml:"ignore"`
	Overrides      []Override            `yaml:"overrides"`
	FrontMatter    *bool                 `yaml:"front-matter"`
	Categories     map[string]bool       `yaml:"categories"`
	Files          []string              `yaml:"files"`
	FollowSymlinks bool                  `yaml:"follow-symlinks"`
	MaxInputSize   string                `yaml:"max-input-size"`
	Archetypes     ArchetypesCfg         `yaml:"archetypes"`
	Kinds          map[string]KindBody   `yaml:"kinds,omitempty"`
	KindAssignment []KindAssignmentEntry `yaml:"kind-assignment,omitempty"`

	// LegacyNoFollowSymlinks captures the removed `no-follow-symlinks`
	// key. Its presence surfaces a deprecation warning via
	// Deprecations; its contents are otherwise ignored now that
	// symlinks are skipped by default. The `omitempty` tag keeps
	// round-tripped configs from re-emitting the deprecated key
	// unless a user explicitly supplied it.
	LegacyNoFollowSymlinks []string `yaml:"no-follow-symlinks,omitempty"`

	// ExplicitRules tracks rule names that were explicitly set in
	// the user config (not just inherited from defaults). This is
	// used for category override resolution: an explicitly enabled
	// rule takes precedence over a disabled category.
	// Not serialized to YAML.
	ExplicitRules map[string]bool `yaml:"-"`

	// FilesExplicit tracks whether the files key was explicitly set
	// in the user config. This distinguishes between an omitted key
	// (use defaults) and an explicitly empty list (no files).
	// Not serialized to YAML.
	FilesExplicit bool `yaml:"-"`

	// Deprecations lists human-readable warnings about deprecated
	// keys found in the loaded config. Callers (cmd/mdsmith) print
	// them to stderr.
	// Not serialized to YAML.
	Deprecations []string `yaml:"-"`
}

// ArchetypesCfg configures archetype discovery. Roots are directories
// searched in order; earlier roots shadow later ones.
type ArchetypesCfg struct {
	Roots []string `yaml:"roots"`
}

// Override applies rule settings to files matching glob patterns.
type Override struct {
	Files      []string           `yaml:"files"`
	Rules      map[string]RuleCfg `yaml:"rules"`
	Categories map[string]bool    `yaml:"categories"`
}

// KindBody is a named bundle of rule settings. It has the same shape as
// Override minus the Files field; files are bound to kinds separately via
// front-matter kinds: or kind-assignment:.
type KindBody struct {
	Rules      map[string]RuleCfg `yaml:"rules"`
	Categories map[string]bool    `yaml:"categories"`
}

// KindAssignmentEntry assigns one or more kinds to files matching glob
// patterns. The glob syntax is the same as overrides: and ignore:.
type KindAssignmentEntry struct {
	Files []string `yaml:"files"`
	Kinds []string `yaml:"kinds"`
}

// RuleCfg is a YAML union: can be bool (enable/disable) or map[string]any (settings).
type RuleCfg struct {
	Enabled  bool
	Settings map[string]any
}

// UnmarshalYAML implements custom YAML unmarshalling for RuleCfg.
// It handles three forms:
//   - false -> Enabled=false, Settings=nil
//   - true  -> Enabled=true,  Settings=nil
//   - {key: val, ...} -> Enabled=true, Settings={key: val, ...}
func (r *RuleCfg) UnmarshalYAML(value *yaml.Node) error {
	// Try bool first
	if value.Kind == yaml.ScalarNode {
		var b bool
		if err := value.Decode(&b); err == nil {
			r.Enabled = b
			r.Settings = nil
			return nil
		}
	}

	// Try map
	if value.Kind == yaml.MappingNode {
		var m map[string]any
		if err := value.Decode(&m); err != nil {
			return fmt.Errorf("invalid rule config: %w", err)
		}
		r.Enabled = true
		r.Settings = m
		return nil
	}

	return fmt.Errorf("rule config must be a bool or a mapping, got %v", value.Kind)
}

// MarshalYAML implements custom YAML marshalling for RuleCfg.
// A disabled rule (Enabled=false, no Settings) serializes as `false`.
// An enabled rule with settings serializes as the settings mapping.
// An enabled rule with no settings serializes as `true`.
func (r RuleCfg) MarshalYAML() (any, error) {
	if !r.Enabled && r.Settings == nil {
		return false, nil
	}
	if r.Enabled && len(r.Settings) > 0 {
		return r.Settings, nil
	}
	return true, nil
}
