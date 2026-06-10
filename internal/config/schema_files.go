package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// schemaFilesDir is the directory under the workspace root that holds
// one YAML file per named schema. The basename of each file (minus
// extension) is the schema's name. Both `*.yaml` and `*.yml` are
// scanned. Subdirectories under this path are rejected at load time —
// one schema per file, no nesting. See plan 241.
const schemaFilesDir = ".mdsmith/schemas"

// schemaFileBasenameRE is the basename pattern a schema file must
// match (the same shape as a kind or convention file: a lowercase
// identifier usable as a registry key, anchored for OS case-folding
// and path safety). It is kept identical to a named `schema:`
// reference (schemaNameRE) so an inline `schema: foo` and a
// `.mdsmith/schemas/foo.yaml` name the same entry.
var schemaFileBasenameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// schemaTopLevelKeys is the set of top-level keys a schema file may
// declare. It is the per-file split of an inline `schemas.<name>:`
// block: the same keys schema.ParseInline reads. Anything else is a
// config error so a typo (e.g. `section:` for `sections:`) surfaces at
// load rather than being silently dropped. Kept in sync with the
// keys parse_inline.go accepts.
var schemaTopLevelKeys = map[string]bool{
	"frontmatter":      true,
	"filename":         true,
	"closed":           true,
	"sections":         true,
	"cross-references": true,
	"acronyms":         true,
	"index":            true,
}

// discoveredSchema pairs a parsed schema body (the raw map that
// schema.ParseInline consumes) with the absolute path of the file it
// came from. The path feeds the inline-vs-file collision check in
// mergeSchemaFiles and the schema-source provenance surface.
type discoveredSchema struct {
	body       map[string]any
	sourcePath string
}

// discoverSchemas walks `.mdsmith/schemas/*.{yaml,yml}` at the
// workspace root and returns one entry per discovered schema. The
// returned map is keyed by basename (the schema's name).
//
// Errors fired (each names the offending file so the user can jump
// straight to it):
//   - basename does not match `[a-z][a-z0-9-]*`
//   - a subdirectory or symlink exists under `.mdsmith/schemas/`
//   - the same basename appears as both `.yaml` and `.yml`
//   - the YAML body has a top-level key outside the schema vocabulary
//   - the file is empty (an empty schema constrains nothing)
//
// A missing or empty `.mdsmith/schemas/` directory returns an empty
// map and no error so callers can blindly merge the result.
func discoverSchemas(workspaceDir string) (map[string]discoveredSchema, error) {
	root := filepath.Join(workspaceDir, schemaFilesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", schemaFilesDir, err)
	}

	// Sort so error messages and the resulting map iteration produce a
	// deterministic order across runs and platforms.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	result := make(map[string]discoveredSchema, len(entries))
	// Track which extension supplied each basename so a later `.yml`
	// colliding with an earlier `.yaml` (or vice versa) can be reported
	// with both filenames.
	seenExt := make(map[string]string, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		// Reject symlinks outright: a symlink reports IsDir()==false
		// from lstat, so without this guard a symlinked directory would
		// slip past the subdirectory check below and a symlinked file
		// would be read off the workspace by parseSchemaFile.
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"%s: symlinks are not allowed (found %q)",
				schemaFilesDir, name)
		}
		if entry.IsDir() {
			return nil, fmt.Errorf(
				"%s: subdirectories are not allowed (found %q)",
				schemaFilesDir, name)
		}
		// Match `.yaml`/`.yml` case-insensitively so a `.YAML` file is
		// not silently skipped (surprising on case-insensitive
		// filesystems where it denotes the same path as `.yaml`).
		ext := filepath.Ext(name)
		switch strings.ToLower(ext) {
		case ".yaml", ".yml":
		default:
			continue
		}
		base := name[:len(name)-len(ext)]
		if !schemaFileBasenameRE.MatchString(base) {
			return nil, fmt.Errorf(
				"%s/%s: basename %q must match %s",
				schemaFilesDir, name, base, schemaFileBasenameRE.String())
		}
		if prior, ok := seenExt[base]; ok {
			return nil, fmt.Errorf(
				"%s: schema %q is declared by both %s and %s; keep one",
				schemaFilesDir, base, prior, name)
		}
		seenExt[base] = name

		path := filepath.Join(root, name)
		body, err := parseSchemaFile(path)
		if err != nil {
			return nil, err
		}
		result[base] = discoveredSchema{body: body, sourcePath: path}
	}
	return result, nil
}

// parseSchemaFile reads one schema file and decodes it into the raw
// body map schema.ParseInline consumes. Unlike kind and convention
// files, a schema file has no Go struct to decode against, so the
// allowed-top-level-key check is enforced explicitly here: a key
// outside schemaTopLevelKeys is a config error naming the file and the
// key. RejectYAMLAliases handles anchor/alias rejection before the
// decode; readLimitedConfig caps the read at maxConfigBytes (1 MB),
// the same guard `.mdsmith.yml` and kind/convention files get.
func parseSchemaFile(path string) (map[string]any, error) {
	data, err := readLimitedConfig(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yamlutil.RejectYAMLAliases(data); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	var body map[string]any
	if err := yamlutil.UnmarshalSafe(data, &body); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// An empty, whitespace-only, or comments-only file decodes to a nil
	// map. An empty schema constrains nothing, so report it clearly
	// rather than registering a no-op entry.
	if len(body) == 0 {
		return nil, fmt.Errorf("%s: empty schema file", path)
	}
	// Validate the top-level vocabulary in sorted key order so the
	// reported error is deterministic when more than one unknown key is
	// present.
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !schemaTopLevelKeys[k] {
			return nil, fmt.Errorf(
				"%s: unknown top-level key %q in schema "+
					"(allowed: %s)",
				path, k, allowedSchemaKeysList())
		}
	}
	return body, nil
}

// mergeSchemaFiles builds the named-schema registry from two sources:
// inline `schemas:` entries (tagged with cfgPath for provenance) and
// file-defined schemas discovered under `.mdsmith/schemas/` (tagged
// with their `.yaml`/`.yml` path). A name declared both inline AND as
// a file is a config error naming both sources — the same rule kinds
// and conventions carry; the two do not merge.
//
// Load is the only caller and always supplies a non-empty cfgPath.
// ParseBytes skips this function (no disk discovery), resolving named
// refs against the inline registry alone via resolveInlineRegistry.
func mergeSchemaFiles(cfg *Config, cfgPath string) (map[string]discoveredSchema, error) {
	reg := resolveInlineRegistry(cfg, cfgPath)

	discovered, err := discoverSchemas(filepath.Dir(cfgPath))
	if err != nil {
		return nil, err
	}
	if len(discovered) == 0 {
		return reg, nil
	}
	if reg == nil {
		reg = make(map[string]discoveredSchema, len(discovered))
	}
	// Iterate in sorted name order so that when more than one file
	// schema collides, the reported error is deterministic across runs
	// instead of depending on map iteration order.
	names := make([]string, 0, len(discovered))
	for name := range discovered {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ds := discovered[name]
		if existing, clash := reg[name]; clash {
			return nil, fmt.Errorf(
				"schema %q is declared both inline in %s and in %s; "+
					"keep one source",
				name, existing.sourcePath, ds.sourcePath)
		}
		reg[name] = ds
	}
	return reg, nil
}

// resolveInlineRegistry seeds the schema registry with the inline
// `schemas:` entries, tagging each with cfgPath so provenance
// attributes an inline-registry schema to `.mdsmith.yml`. Returns nil
// when no inline schemas are declared so the caller can size the map
// to the discovered count.
func resolveInlineRegistry(cfg *Config, cfgPath string) map[string]discoveredSchema {
	if len(cfg.Schemas) == 0 {
		return nil
	}
	reg := make(map[string]discoveredSchema, len(cfg.Schemas))
	for name, body := range cfg.Schemas {
		reg[name] = discoveredSchema{body: body, sourcePath: cfgPath}
	}
	return reg
}

// resolveNamedSchemas replaces every kind's named `schema:` reference
// with the body from the registry, stamping the ref's SourcePath from
// the entry's origin (a `.yaml` path for a file schema, `.mdsmith.yml`
// for an inline-registry schema). A reference to an undeclared name is
// a config error naming the kind and the missing schema. Inline bodies
// (refs with no Name) pass through untouched, keeping an empty
// SourcePath so the kind's own file applies as the schema source.
//
// Kinds are walked in sorted name order so an undeclared-name error is
// deterministic when several kinds reference missing schemas.
func resolveNamedSchemas(cfg *Config, reg map[string]discoveredSchema) error {
	if len(cfg.Kinds) == 0 {
		return nil
	}
	names := make([]string, 0, len(cfg.Kinds))
	for name := range cfg.Kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := cfg.Kinds[name]
		if body.Schema.Name == "" {
			continue
		}
		ds, ok := reg[body.Schema.Name]
		if !ok {
			return fmt.Errorf(
				"kind %q: schema %q is not declared inline under "+
					"`schemas:` or as a file under %s",
				name, body.Schema.Name, schemaFilesDir)
		}
		body.Schema = resolvedSchemaRef(body.Schema.Name, ds.body, ds.sourcePath)
		cfg.Kinds[name] = body
	}
	return nil
}

// allowedSchemaKeysList renders the schema vocabulary as a sorted,
// comma-separated list for the unknown-key error message.
func allowedSchemaKeysList() string {
	keys := make([]string, 0, len(schemaTopLevelKeys))
	for k := range schemaTopLevelKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
