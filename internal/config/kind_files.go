package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

// kindFilesDir is the directory under the workspace root that
// holds one YAML file per kind. The basename of each file (minus
// extension) is the kind name. Both `*.yaml` and `*.yml` are
// scanned. Subdirectories under this path are rejected at load
// time — one kind per file, no nesting. See plan 208.
const kindFilesDir = ".mdsmith/kinds"

// kindFileBasenameRE is the basename pattern a kind file must
// match (the same shape as a YAML map key but anchored for OS
// case-folding and path safety). Inline `kinds.<name>:` keys stay
// unvalidated — this constraint only applies to filenames.
var kindFileBasenameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// discoveredKind pairs a parsed KindBody with the absolute path
// of the file it came from. The path feeds the dual-source
// collision check in Load and the per-kind provenance surface.
type discoveredKind struct {
	body       KindBody
	sourcePath string
}

// discoverKinds walks `.mdsmith/kinds/*.{yaml,yml}` at the
// workspace root and returns one entry per discovered kind. The
// returned map is keyed by basename (the kind's name).
//
// Errors fired (each names the offending file so the user can
// jump straight to it):
//   - basename does not match `[a-z][a-z0-9-]*`
//   - a subdirectory exists under `.mdsmith/kinds/`
//   - the same basename appears as both `.yaml` and `.yml`
//   - the YAML body has a top-level key outside `KindBody`
//
// A missing or empty `.mdsmith/kinds/` directory returns an
// empty map and no error so callers can blindly merge the
// result.
func discoverKinds(workspaceDir string) (map[string]discoveredKind, error) {
	root := filepath.Join(workspaceDir, kindFilesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", kindFilesDir, err)
	}

	// Sort so error messages and the resulting map iteration
	// produce a deterministic order across runs and platforms.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	result := make(map[string]discoveredKind, len(entries))
	// Track which extension supplied each basename so a later
	// `.yml` colliding with an earlier `.yaml` (or vice versa)
	// can be reported with both filenames.
	seenExt := make(map[string]string, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			return nil, fmt.Errorf(
				"%s: subdirectories are not allowed (found %q)",
				kindFilesDir, name)
		}
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		base := name[:len(name)-len(ext)]
		if !kindFileBasenameRE.MatchString(base) {
			return nil, fmt.Errorf(
				"%s/%s: basename %q must match %s",
				kindFilesDir, name, base, kindFileBasenameRE.String())
		}
		if prior, ok := seenExt[base]; ok {
			return nil, fmt.Errorf(
				"%s: kind %q is declared by both %s and %s; "+
					"keep one",
				kindFilesDir, base, prior, name)
		}
		seenExt[base] = name

		path := filepath.Join(root, name)
		body, err := parseKindFile(path)
		if err != nil {
			return nil, err
		}
		body.SourcePath = path
		result[base] = discoveredKind{body: body, sourcePath: path}
	}
	return result, nil
}

// mergeKindFiles tags every inline kind with cfgPath for
// provenance, then discovers file-defined kinds under the
// workspace root (parent of cfgPath) and merges them into
// cfg.Kinds. A name colliding between a file kind and an inline
// kind is a config error naming both sources — the two do not
// merge (a merged kind would defeat the "read one file to know
// one kind" property plan 208 ships). Load is the only caller
// and always supplies a non-empty cfgPath, so no defensive
// guard is needed for that.
func mergeKindFiles(cfg *Config, cfgPath string) error {
	// Tag every inline kind with cfgPath so provenance attributes
	// kinds uniformly whether they came from `.mdsmith.yml` or a file
	// under `.mdsmith/kinds/`. This runs before the file merge so a
	// collision diagnostic can quote both sources verbatim.
	for name, body := range cfg.Kinds {
		body.SourcePath = cfgPath
		cfg.Kinds[name] = body
	}

	discovered, err := discoverKinds(filepath.Dir(cfgPath))
	if err != nil {
		return err
	}
	if len(discovered) == 0 {
		return nil
	}
	if cfg.Kinds == nil {
		cfg.Kinds = make(map[string]KindBody, len(discovered))
	}
	for name, dk := range discovered {
		if existing, clash := cfg.Kinds[name]; clash {
			return fmt.Errorf(
				"kind %q is declared both inline in %s and in %s; "+
					"keep one source",
				name, existing.SourcePath, dk.sourcePath)
		}
		cfg.Kinds[name] = dk.body
	}
	return nil
}

// parseKindFile reads one kind file and decodes it into a
// KindBody with strict (KnownFields) decoding so a typo in a
// top-level key surfaces as a config error rather than being
// silently dropped. UnmarshalSafe handles anchor/alias rejection
// before the strict decode runs.
func parseKindFile(path string) (KindBody, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is built from workspace + kindFilesDir
	if err != nil {
		return KindBody{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yamlutil.RejectYAMLAliases(data); err != nil {
		return KindBody{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	var body KindBody
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&body); err != nil {
		return KindBody{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return body, nil
}
