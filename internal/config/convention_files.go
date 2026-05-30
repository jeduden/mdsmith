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

// conventionFilesDir is the directory under the workspace root that
// holds one YAML file per user-defined convention. The basename of
// each file (minus extension) is the convention name. Both `*.yaml`
// and `*.yml` are scanned. Subdirectories under this path are
// rejected at load time — one convention per file, no nesting. See
// plan 209.
const conventionFilesDir = ".mdsmith/conventions"

// conventionFileBasenameRE is the basename pattern a convention file
// must match (the same shape as a kind file: a lowercase identifier
// usable as a YAML map key, anchored for OS case-folding and path
// safety). Inline `conventions.<name>:` keys stay unvalidated — this
// constraint only applies to filenames.
var conventionFileBasenameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// discoveredConvention pairs a parsed UserConvention with the
// absolute path of the file it came from. The path feeds the
// dual-source collision check in Load and the per-convention
// provenance surface.
type discoveredConvention struct {
	body       UserConvention
	sourcePath string
}

// discoverConventions walks `.mdsmith/conventions/*.{yaml,yml}` at
// the workspace root and returns one entry per discovered
// convention. The returned map is keyed by basename (the
// convention's name).
//
// Errors fired (each names the offending file so the user can jump
// straight to it):
//   - basename does not match `[a-z][a-z0-9-]*`
//   - a subdirectory exists under `.mdsmith/conventions/`
//   - the same basename appears as both `.yaml` and `.yml`
//   - the YAML body has a top-level key outside UserConvention
//
// A missing or empty `.mdsmith/conventions/` directory returns an
// empty map and no error so callers can blindly merge the result.
func discoverConventions(workspaceDir string) (map[string]discoveredConvention, error) {
	root := filepath.Join(workspaceDir, conventionFilesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", conventionFilesDir, err)
	}

	// Sort so error messages and the resulting map iteration produce
	// a deterministic order across runs and platforms.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	result := make(map[string]discoveredConvention, len(entries))
	// Track which extension supplied each basename so a later `.yml`
	// colliding with an earlier `.yaml` (or vice versa) can be
	// reported with both filenames.
	seenExt := make(map[string]string, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			return nil, fmt.Errorf(
				"%s: subdirectories are not allowed (found %q)",
				conventionFilesDir, name)
		}
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		base := name[:len(name)-len(ext)]
		if !conventionFileBasenameRE.MatchString(base) {
			return nil, fmt.Errorf(
				"%s/%s: basename %q must match %s",
				conventionFilesDir, name, base, conventionFileBasenameRE.String())
		}
		if prior, ok := seenExt[base]; ok {
			return nil, fmt.Errorf(
				"%s: convention %q is declared by both %s and %s; "+
					"keep one",
				conventionFilesDir, base, prior, name)
		}
		seenExt[base] = name

		path := filepath.Join(root, name)
		body, err := parseConventionFile(path)
		if err != nil {
			return nil, err
		}
		body.SourcePath = path
		result[base] = discoveredConvention{body: body, sourcePath: path}
	}
	return result, nil
}

// parseConventionFile reads one convention file and decodes it into a
// UserConvention with strict (KnownFields) decoding so a typo in a
// top-level key surfaces as a config error rather than being silently
// dropped. RejectYAMLAliases handles anchor/alias rejection before the
// strict decode runs.
func parseConventionFile(path string) (UserConvention, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is built from workspace + conventionFilesDir
	if err != nil {
		return UserConvention{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yamlutil.RejectYAMLAliases(data); err != nil {
		return UserConvention{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	var body UserConvention
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&body); err != nil {
		return UserConvention{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return body, nil
}
