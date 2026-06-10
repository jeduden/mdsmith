package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ValidCategories lists the recognized rule category names.
var ValidCategories = []string{
	"accessibility",
	"code",
	"directive",
	"heading",
	"line",
	"link",
	"list",
	"prose",
	"structural",
	"table",
	"whitespace",
}

// DefaultFiles is the built-in list of glob patterns used for file
// discovery when no file arguments are given on the command line.
var DefaultFiles = []string{"**/*.md", "**/*.markdown"}

// UserConvention is a user-defined convention bundle declared either
// inline under the top-level `conventions:` block in .mdsmith.yml or
// in a standalone file under `.mdsmith/conventions/<name>.yaml` (plan
// 209). It has the same { flavor, rules } shape as the built-in
// convention table.
type UserConvention struct {
	// Flavor is the Markdown flavor MDS034 should validate against
	// (e.g. "commonmark", "gfm", "goldmark").
	Flavor string `yaml:"flavor"`
	// Rules maps rule names to their presets, using the same schema
	// as the top-level `rules:` block.
	Rules map[string]RuleCfg `yaml:"rules,omitempty"`

	// SourcePath is the workspace-absolute path of the file that
	// defined this convention — either `.mdsmith.yml` for inline
	// conventions or `.mdsmith/conventions/<name>.{yaml,yml}` for
	// file-defined conventions (plan 209). Not serialized to YAML;
	// populated by Load. Provenance surfaces it on the convention
	// layer; CLI resolve/why prints it next to the layer.
	SourcePath string `yaml:"-"`
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
	// convention. See internal/convention/convention.go and
	// docs/reference/conventions.md.
	Convention string `yaml:"convention,omitempty"`

	// Conventions holds user-defined convention bundles declared
	// under the top-level `conventions:` key. Each entry is a
	// { flavor, rules } pair. Names must not collide with the
	// built-in conventions ("portable", "github", "plain").
	Conventions map[string]UserConvention `yaml:"conventions,omitempty"`

	// Schemas holds named document-structure schemas declared inline
	// under the top-level `schemas:` key — the inline equivalent of a
	// `.mdsmith/schemas/<name>.yaml` file (plan 241). Each entry is a
	// raw schema body (the map schema.ParseInline consumes). A kind
	// references one by name (`schema: rfc-v1`); resolveNamedSchemas
	// replaces the reference with the body at load time. Names must
	// not collide with a `.mdsmith/schemas/` file basename.
	Schemas map[string]map[string]any `yaml:"schemas,omitempty"`

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
// of this kind. It is polymorphic (see KindSchemaRef): a YAML scalar
// names an entry in the top-level `schemas:` registry (resolved to a
// body at load time by resolveNamedSchemas), while a YAML mapping is
// an inline body carried directly on the kind. Either form is an
// alternative to referencing a proto.md file via
// rules.required-structure.schema:. A kind that sets Schema and
// rules.required-structure.schema: (or inline-schema:) is a config
// error; see ValidateKinds. Read the resolved body via Schema.Map().
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
	Schema      KindSchemaRef      `yaml:"schema,omitempty"`
	PathPattern string             `yaml:"path-pattern,omitempty"`
	// Extends, when set, names another kind whose `schema:` this
	// kind inherits from. Frontmatter constraints unify under CUE
	// (the child's expression for a shared key joins the parent's
	// via `&`); sections in the child wholly replace the parent's
	// sections. A cycle in the extends chain (single- or multi-hop)
	// is reported by ValidateKinds. See plan 135.
	Extends string `yaml:"extends,omitempty"`

	// SourcePath is the workspace-absolute path of the file that
	// defined this kind — either `.mdsmith.yml` for inline kinds
	// or `.mdsmith/kinds/<name>.{yaml,yml}` for file-defined kinds
	// (plan 208). Not serialized to YAML; populated by Load.
	// Provenance surfaces it as `kinds.<name> (<path>)`; CLI
	// resolve/show prints it next to each kind.
	SourcePath string `yaml:"-"`
}

// KindAssignmentEntry assigns one or more kinds to files matching the
// configured selectors. Two selectors are available: a glob set
// (`glob:`) and a front-matter field-presence set (`fields-present:`).
// Within a single entry the selectors combine with AND — every set
// selector must match. Across entries the matches union (OR semantics).
type KindAssignmentEntry struct {
	// Glob is the canonical field for file patterns (doublestar syntax,
	// supports **, brace expansion, and !-prefix exclusion).
	Glob []string `yaml:"glob,omitempty"`
	// Files is a deprecated alias for Glob. Use Glob in new configs.
	Files []string `yaml:"files,omitempty"`
	// FieldsPresent lists front-matter keys that must all be present
	// with a non-null value for this entry to match. An empty list
	// disables the selector — only glob is considered.
	FieldsPresent []string `yaml:"fields-present,omitempty"`
	Kinds         []string `yaml:"kinds"`
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
