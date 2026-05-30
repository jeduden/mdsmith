package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/convention"
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
//   - a subdirectory or symlink exists under `.mdsmith/conventions/`
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
		// Reject symlinks outright: a symlink reports IsDir()==false
		// from lstat, so without this guard a symlinked directory
		// would slip past the subdirectory check below and a symlinked
		// file would be read off the workspace by parseConventionFile.
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"%s: symlinks are not allowed (found %q)",
				conventionFilesDir, name)
		}
		if entry.IsDir() {
			return nil, fmt.Errorf(
				"%s: subdirectories are not allowed (found %q)",
				conventionFilesDir, name)
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

// mergeConventionFiles tags every inline convention with cfgPath for
// provenance, then discovers file-defined conventions under the
// workspace root (parent of cfgPath) and merges them into
// cfg.Conventions. Two collisions are config errors, each naming the
// offending file so the user can resolve it:
//
//   - A name colliding between a file convention and an inline
//     convention names both sources — the two do not merge (a merged
//     convention would defeat the "read one file to know one
//     convention" property this plan ships).
//   - A name colliding with a built-in convention (portable, github,
//     plain, …) names the file and reports the name as reserved.
//
// Load is the only caller and always supplies a non-empty cfgPath, so
// no defensive guard is needed for that.
func mergeConventionFiles(cfg *Config, cfgPath string) error {
	// Tag every inline convention with cfgPath so provenance
	// attributes a user convention uniformly whether it came from
	// `.mdsmith.yml` or a file under `.mdsmith/conventions/`. This
	// runs before the file merge so a collision diagnostic can quote
	// both sources verbatim.
	for name, uc := range cfg.Conventions {
		uc.SourcePath = cfgPath
		cfg.Conventions[name] = uc
	}

	discovered, err := discoverConventions(filepath.Dir(cfgPath))
	if err != nil {
		return err
	}
	if len(discovered) == 0 {
		return nil
	}

	reserved := make(map[string]bool, len(convention.Names()))
	for _, name := range convention.Names() {
		reserved[name] = true
	}

	if cfg.Conventions == nil {
		cfg.Conventions = make(map[string]UserConvention, len(discovered))
	}
	// Iterate in sorted name order so that when more than one file
	// convention is in conflict, the reported error is deterministic
	// across runs instead of depending on map iteration order.
	names := make([]string, 0, len(discovered))
	for name := range discovered {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dc := discovered[name]
		if reserved[name] {
			return fmt.Errorf(
				"convention %q in %s: name is reserved by a built-in convention",
				name, dc.sourcePath)
		}
		if existing, clash := cfg.Conventions[name]; clash {
			return fmt.Errorf(
				"convention %q is declared both inline in %s and in %s; "+
					"keep one source",
				name, existing.SourcePath, dc.sourcePath)
		}
		cfg.Conventions[name] = dc.body
	}
	return nil
}

// parseConventionFile reads one convention file and decodes it into a
// UserConvention with strict (KnownFields) decoding so a typo in a
// top-level key surfaces as a config error rather than being silently
// dropped. RejectYAMLAliases handles anchor/alias rejection before the
// strict decode runs.
func parseConventionFile(path string) (UserConvention, error) {
	// readLimitedConfig caps the read at maxConfigBytes (1 MB), the
	// same guard `.mdsmith.yml` gets — a convention file should never
	// be large, and an unbounded os.ReadFile is a needless OOM surface.
	data, err := readLimitedConfig(path)
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
		// An empty, whitespace-only, or comments-only file decodes to
		// io.EOF (no YAML node). A convention with no body can't be
		// validated downstream (applyConvention requires a flavor), so
		// report it clearly instead of surfacing the decoder's bare
		// "EOF".
		if errors.Is(err, io.EOF) {
			return UserConvention{}, fmt.Errorf("%s: empty convention file", path)
		}
		return UserConvention{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return body, nil
}
