package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

// maxConfigBytes is a generous cap for the config file size (1 MB).
// Config files should be small; this prevents accidental OOM from
// pointing at a huge file.
const maxConfigBytes int64 = 1024 * 1024

const configFileName = ".mdsmith.yml"

// pyprojectFileName is the Python project file whose `[tool.mdsmith]`
// table Discover accepts as an alternative config source.
const pyprojectFileName = "pyproject.toml"

// DefaultConfigPath returns the default config path under dir (the
// conventional .mdsmith.yml). It is the single source of truth for the
// config filename outside this package.
func DefaultConfigPath(dir string) string {
	return filepath.Join(dir, configFileName)
}

// Load reads and parses a config file at the given path. A path with a
// `.toml` extension (pyproject.toml or any other TOML file) is read
// from its `[tool.mdsmith]` table; every other path is YAML. A failure
// is a *LoadError positioned at the offending value when it is known.
func Load(path string) (*Config, error) {
	if isTOMLPath(path) {
		return loadPyproject(path)
	}
	data, err := readLimitedConfig(path)
	if err != nil {
		return nil, positionError(fmt.Errorf("reading config file: %w", err), path, nil)
	}
	cfg, err := loadFromBytes(data, path, true)
	if err != nil {
		return nil, yamlPositionError(err, path, data)
	}
	return cfg, nil
}

// isTOMLPath reports whether path names a TOML config file.
func isTOMLPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".toml")
}

// yamlResolverFor returns a lazy constructor for the YAML resolver over
// data, so the node tree is only built when an error needs a position.
func yamlResolverFor(data []byte) func() PositionResolver {
	return func() PositionResolver { return newYAMLResolver(data) }
}

// ParseBytes parses config from an in-memory YAML byte slice, running
// the same convention/validation pipeline as Load but reading no disk:
// disk-based kind-file discovery (`.mdsmith/kinds/`) is skipped because
// an in-memory config carries every kind inline. It is the entry point
// the public engine session uses for an inline `configYAML` (the WASM
// path), mirroring how the `-c` flag's file text is processed. Empty
// input yields a usable, mostly-default Config.
func ParseBytes(data []byte) (*Config, error) {
	cfg, err := loadFromBytes(data, "", false)
	if err != nil {
		return nil, yamlPositionError(err, "", data)
	}
	return cfg, nil
}

// loadFromBytes is the shared parse pipeline behind Load and
// ParseBytes. sourcePath tags inline kinds and conventions for
// provenance and anchors disk discovery of `.mdsmith/{kinds,
// conventions}/`; mergeKinds gates those disk reads so the in-memory
// path stays filesystem-free.
func loadFromBytes(data []byte, sourcePath string, mergeKinds bool) (*Config, error) {
	doc, err := yamlutil.UnmarshalNodeSafe(data)
	if err != nil {
		return nil, fmt.Errorf("parsing config file: %w", yamlErrorIssue(err))
	}
	return loadFromNode(&doc, sourcePath, mergeKinds)
}

// loadFromNode runs the config pipeline over an alias-free YAML
// document node: the parsed `.mdsmith.yml`, or the node tree built
// from a pyproject `[tool.mdsmith]` table. A zero node (empty input)
// yields a mostly-default Config.
func loadFromNode(doc *yaml.Node, sourcePath string, mergeKinds bool) (*Config, error) {
	// Catch non-string `convention:` values before decoding silently
	// coerces them into the string field.
	if err := validateConventionNode(doc); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	var cfg Config
	if doc.Kind != 0 {
		if err := yamlutil.DecodeNodeSafe(doc, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", yamlErrorIssue(err))
		}
	}

	// Detect top-level key presence so "files" (omitted vs empty) and
	// deprecated keys can be probed without re-parsing per key.
	keys := topLevelKeys(doc)
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

	// Merge file-defined kinds, conventions, and word-lists from the
	// `.mdsmith/` resource directories (plans 208, 209). Gated by
	// mergeKinds so the in-memory ParseBytes path performs no
	// filesystem discovery.
	if mergeKinds {
		if err := mergeFileResources(&cfg, sourcePath); err != nil {
			return nil, err
		}
	}

	if err := mergeAndResolveSchemas(&cfg, sourcePath, mergeKinds); err != nil {
		return nil, err
	}

	if err := validateConfigSemantics(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	if err := checkBuildConfig(doc, &cfg); err != nil {
		return nil, err
	}

	if err := applyConvention(&cfg); err != nil {
		return nil, fmt.Errorf("applying convention: %w", err)
	}

	// Runs after applyConvention so the convention preset's `lists:`
	// references are validated alongside the user's own.
	if err := validateWordlists(&cfg); err != nil {
		return nil, fmt.Errorf("validating wordlists: %w", err)
	}

	return &cfg, nil
}

// mergeFileResources merges file-defined kinds, conventions, and
// word-lists from `.mdsmith/{kinds,conventions,wordlists}/`. Each merge
// tags inline entries with sourcePath for provenance and errors on name
// collisions.
func mergeFileResources(cfg *Config, sourcePath string) error {
	if err := mergeKindFiles(cfg, sourcePath); err != nil {
		return fmt.Errorf("loading kind files: %w", err)
	}
	if err := mergeConventionFiles(cfg, sourcePath); err != nil {
		return fmt.Errorf("loading convention files: %w", err)
	}
	if err := mergeWordlistFiles(cfg, sourcePath); err != nil {
		return fmt.Errorf("loading wordlist files: %w", err)
	}
	return nil
}

// mergeAndResolveSchemas builds the named-schema registry, then
// resolves each kind's named `schema:` reference to a body, so
// ValidateKinds and the merge layer see one inline body — exactly as
// they do for an inline-on-kind schema. The registry combines inline
// `schemas:` entries with file-defined schemas under
// `.mdsmith/schemas/` (plan 241). When mergeKinds is false (the
// in-memory ParseBytes path), disk discovery is skipped and the
// resolver runs against the inline registry alone.
func mergeAndResolveSchemas(cfg *Config, sourcePath string, mergeKinds bool) error {
	var reg map[string]discoveredSchema
	if mergeKinds {
		merged, err := mergeSchemaFiles(cfg, sourcePath)
		if err != nil {
			return fmt.Errorf("loading schema files: %w", err)
		}
		reg = merged
	} else {
		reg = resolveInlineRegistry(cfg, sourcePath)
	}
	if err := resolveNamedSchemas(cfg, reg); err != nil {
		return fmt.Errorf("resolving schemas: %w", err)
	}
	return nil
}

// topLevelKeys returns the set of top-level mapping keys of a document
// node, or nil when its root is not a mapping.
func topLevelKeys(node *yaml.Node) map[string]bool {
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return nil
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	result := make(map[string]bool, len(mapping.Content)/2)
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		result[mapping.Content[i].Value] = true
	}
	return result
}

// checkBuildConfig runs the two build-config validators that must run
// after the main YAML parse but before convention application. It is
// extracted from loadFromBytes to keep that function under the funlen
// limit.
func checkBuildConfig(doc *yaml.Node, cfg *Config) error {
	if err := rejectRemovedBuildKeys(doc); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}
	if err := ValidateBuildConfig(cfg); err != nil {
		return fmt.Errorf("validating config: %w", err)
	}
	return nil
}

// rejectRemovedBuildKeys errors if the config still carries a
// `build.base-url:` key. The struct field was removed in plan
// 2606101546, and non-strict YAML (yamlutil.UnmarshalSafe) would
// otherwise drop the key silently, leaving an author to wonder why their
// setting has no effect. The scan walks the `build:` mapping node
// directly because base-url is nested, not top-level.
func rejectRemovedBuildKeys(node *yaml.Node) error {
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return nil
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != "build" {
			continue
		}
		buildNode := mapping.Content[i+1]
		if buildNode.Kind != yaml.MappingNode {
			return nil
		}
		for j := 0; j+1 < len(buildNode.Content); j += 2 {
			if buildNode.Content[j].Value == "base-url" {
				return issueAt(KeyPath{"build", "base-url"},
					"build.base-url was removed in plan 2606101546; delete it")
			}
		}
	}
	return nil
}

// FileIn returns the config file that lives directly in dir, or
// "" when there is none: dir/.mdsmith.yml when it exists, else
// dir/pyproject.toml when it holds a `[tool.mdsmith]` table. It is the
// per-directory rule Discover applies at each step of its walk, shared
// with callers that read one known directory's config (such as the
// merge-driver glob set at the repository root).
func FileIn(dir string) string {
	found, _ := fileIn(dir)
	return found
}

// fileIn is FileIn plus, when dir holds no config file, the hint for a
// pyproject.toml there whose table is the plural `[tools.mdsmith]`. The
// pyproject.toml is read and parsed once for both answers.
func fileIn(dir string) (found, hint string) {
	candidate := filepath.Join(dir, configFileName)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, ""
	}
	py := filepath.Join(dir, pyprojectFileName)
	source, hint := probePyproject(py)
	if !source {
		return "", hint
	}
	return py, ""
}

// IsConfigFile reports whether path names a file that can be an mdsmith
// config source: a .mdsmith.yml or a pyproject.toml. Watchers use it to
// decide when a file change must reload config.
func IsConfigFile(path string) bool {
	base := filepath.Base(path)
	return base == configFileName || base == pyprojectFileName
}

// sidecarDirs are the directories, beside a config file, whose YAML
// files Load merges into the config.
var sidecarDirs = [...]string{kindFilesDir, conventionFilesDir, schemaFilesDir, wordlistFilesDir}

// SidecarOwnerDir reports whether path is a kind, convention, schema or
// word-list file — a .yml or .yaml file directly in one of the
// .mdsmith/ sidecar directories — and returns the directory whose
// config file Load reads it beside. Watchers use it so that editing a
// sidecar reloads config the way editing the config file does.
func SidecarOwnerDir(path string) (string, bool) {
	switch filepath.Ext(path) {
	case ".yml", ".yaml":
	default:
		return "", false
	}
	dir := filepath.Dir(path)
	owner := filepath.Dir(filepath.Dir(dir))
	for _, sub := range sidecarDirs {
		if dir == filepath.Join(owner, sub) {
			return owner, true
		}
	}
	return "", false
}

// Discover walks up the directory tree from startDir looking for a
// config file: a .mdsmith.yml, or a pyproject.toml that holds a
// `[tool.mdsmith]` table. Within one directory .mdsmith.yml wins; across
// directories the nearest file wins; a pyproject.toml without the table
// is not a config source and the walk continues past it. It stops
// searching when it encounters a .git directory (the repository root)
// or reaches the filesystem root. Returns the path to the config file,
// or "" if none was found.
func Discover(startDir string) (string, error) {
	found, _ := DiscoverWithHints(startDir)
	return found, nil
}

// DiscoverWithHints is Discover plus the one-line hints its walk
// collects for files it passed over that look like a misplaced config:
// a pyproject.toml whose table is the plural `[tools.mdsmith]`. Callers
// print the hints so the author learns why the table is not read. One
// walk reads each pyproject.toml once for both answers.
func DiscoverWithHints(startDir string) (found string, hints []string) {
	dir, _ := filepath.Abs(startDir) // filepath.Abs cannot fail when os.Getwd succeeds
	for {
		f, hint := fileIn(dir)
		if f != "" {
			return f, hints
		}
		if hint != "" {
			hints = append(hints, hint)
		}

		// Check for .git boundary — if .git exists in this dir,
		// this is the repo root and we should not search further up.
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return "", hints
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", hints
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

// statFileSize returns the open file's size, or -1 if Stat fails. It is a
// var so a test can force the Stat-failure path of readLimitedConfig's
// over-limit size report, which a real filesystem never reaches once Open
// has succeeded.
var statFileSize = func(f *os.File) int64 {
	info, err := f.Stat()
	if err != nil {
		return -1
	}
	return info.Size()
}

// readLimitedConfig reads a config file with a size cap to prevent OOM.
func readLimitedConfig(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	// Stat for accurate size in error messages.
	actualSize := statFileSize(f)

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

// hasMetaCategory reports whether a meta key exists in any categories block
// in the config (top-level, kinds, or overrides).
func hasMetaCategory(cfg *Config) bool {
	if _, ok := cfg.Categories["meta"]; ok {
		return true
	}
	for _, kind := range cfg.Kinds {
		if _, ok := kind.Categories["meta"]; ok {
			return true
		}
	}
	for _, o := range cfg.Overrides {
		if _, ok := o.Categories["meta"]; ok {
			return true
		}
	}
	return false
}

func detectMetaCategoryDeprecations(cfg *Config) {
	if !hasMetaCategory(cfg) {
		return
	}
	cfg.Deprecations = append(cfg.Deprecations,
		"category `meta` no longer exists; rules previously in `meta` now use "+
			"`directive`, `structural`, or `prose` — update your `categories:` block. "+
			"Rules that moved to `prose` (paragraph-readability, paragraph-structure, "+
			"token-budget, conciseness-scoring, duplicated-content, emphasis-style, "+
			"ambiguous-emphasis) must be disabled by rule name, not by category.")
}

func enabledByDefault(r rule.Rule) bool {
	if d, ok := r.(rule.Defaultable); ok {
		return d.EnabledByDefault()
	}
	return true
}
