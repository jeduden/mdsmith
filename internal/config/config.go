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
	"prose",
	"table",
}

// DefaultFiles is the built-in list of glob patterns used for file
// discovery when no file arguments are given on the command line.
var DefaultFiles = []string{"**/*.md", "**/*.markdown"}

// UserConvention is a user-defined convention bundle declared in the
// top-level `conventions:` block in .mdsmith.yml. It has the same
// { flavor, rules } shape as the built-in convention table.
type UserConvention struct {
	// Flavor is the Markdown flavor MDS034 should validate against
	// (e.g. "commonmark", "gfm", "goldmark").
	Flavor string `yaml:"flavor"`
	// Rules maps rule names to their presets, using the same schema
	// as the top-level `rules:` block.
	Rules map[string]RuleCfg `yaml:"rules,omitempty"`
}

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
	Kinds          map[string]KindBody   `yaml:"kinds,omitempty"`
	KindAssignment []KindAssignmentEntry `yaml:"kind-assignment,omitempty"`
	Build          BuildConfig           `yaml:"build,omitempty"`

	// Convention names a Markdown convention bundle. Built-in
	// values: "portable", "github", "plain". User-defined
	// conventions may also be referenced here after being declared
	// under the top-level `conventions:` key. Empty means no
	// convention. See internal/rules/markdownflavor/conventions.go
	// and docs/reference/conventions.md.
	Convention string `yaml:"convention,omitempty"`

	// Conventions holds user-defined convention bundles declared
	// under the top-level `conventions:` key. Each entry is a
	// { flavor, rules } pair. Names must not collide with the
	// built-in conventions ("portable", "github", "plain").
	Conventions map[string]UserConvention `yaml:"conventions,omitempty"`

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

	// ConventionPreset is the convention's rule preset table,
	// captured at config load time. It is applied as a base layer
	// beneath the user's top-level rules: in effective-rule
	// resolution, the preset is merged first, the user's cfg.Rules
	// wins via deep-merge, then kinds and overrides apply on top.
	// Empty when no convention is selected.
	// Not serialized to YAML.
	ConventionPreset map[string]RuleCfg `yaml:"-"`
}

// Override applies rule settings to files matching glob patterns.
type Override struct {
	// Glob is the canonical field for file patterns (doublestar syntax,
	// supports **, brace expansion, and !-prefix exclusion).
	Glob []string `yaml:"glob,omitempty"`
	// Files is a deprecated alias for Glob. Use Glob in new configs.
	Files      []string           `yaml:"files,omitempty"`
	Rules      map[string]RuleCfg `yaml:"rules"`
	Categories map[string]bool    `yaml:"categories"`
}

// Patterns returns the effective set of glob patterns for the override.
// When Glob is set it takes precedence; Files is used only when Glob is
// absent (backward compatibility with the deprecated files: key).
func (o Override) Patterns() []string {
	if len(o.Glob) > 0 {
		return o.Glob
	}
	return o.Files
}

// KindBody is a named bundle of rule settings. It has the same shape as
// Override minus the Files field; files are bound to kinds separately via
// front-matter kinds: or kind-assignment:.
//
// Schema, when set, declares the document-structure schema for files
// of this kind inline (rather than referencing a proto.md file via
// rules.required-structure.schema:). A kind that sets both Schema
// and rules.required-structure.schema: is a config error; see
// ValidateKinds.
//
// PathPattern, when set, is a glob the workspace-relative path of
// every file assigned to this kind must match. The pattern uses the
// doublestar syntax shared by overrides:, ignore:, and
// kind-assignment:, anchored at the workspace root. A path mismatch
// surfaces as an MDS020 diagnostic. PathPattern is the kind-config
// counterpart to the per-schema `<?require filename:?>` directive
// and the two may coexist on the same kind — each runs through its
// own matcher (`<?require filename:?>` uses filepath.Match against
// the basename, PathPattern uses doublestar against the workspace-
// relative path) and emits a separate diagnostic when violated.
type KindBody struct {
	Rules       map[string]RuleCfg `yaml:"rules"`
	Categories  map[string]bool    `yaml:"categories"`
	Schema      map[string]any     `yaml:"schema,omitempty"`
	PathPattern string             `yaml:"path-pattern,omitempty"`
}

// KindAssignmentEntry assigns one or more kinds to files matching glob
// patterns. The glob syntax is the same as overrides: and ignore:.
type KindAssignmentEntry struct {
	// Glob is the canonical field for file patterns (doublestar syntax,
	// supports **, brace expansion, and !-prefix exclusion).
	Glob []string `yaml:"glob,omitempty"`
	// Files is a deprecated alias for Glob. Use Glob in new configs.
	Files []string `yaml:"files,omitempty"`
	Kinds []string `yaml:"kinds"`
}

// Patterns returns the effective set of glob patterns for the entry.
// When Glob is set it takes precedence; Files is used only when Glob is
// absent (backward compatibility with the deprecated files: key).
func (e KindAssignmentEntry) Patterns() []string {
	if len(e.Glob) > 0 {
		return e.Glob
	}
	return e.Files
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
