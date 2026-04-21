// Package archetypes discovers user-supplied required-structure
// archetype schemas from a set of configured root directories.
//
// An archetype is a Markdown schema file whose basename (without the
// ".md" extension) is the archetype name. Resolvers search roots in
// order; earlier roots shadow later roots with the same name.
package archetypes

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry describes a discovered archetype.
type Entry struct {
	Name string // basename without ".md"
	Path string // path relative to the resolver's root directory
	Root string // root directory that contained this archetype
}

// Resolver finds archetypes across a list of root directories.
//
// Paths in Resolver are interpreted relative to RootDir. When RootDir
// is empty, Resolver uses paths as-is relative to the current working
// directory. When FS is non-nil, directory reads use it; otherwise
// os.DirFS(RootDir) is used. This allows tests to inject an in-memory
// filesystem without touching the working directory.
type Resolver struct {
	Roots   []string
	RootDir string
	FS      fs.FS
}

// DefaultRoot is the directory used when no archetype roots are
// configured. It is applied when Resolver.Roots is empty.
const DefaultRoot = "archetypes"

// roots returns the effective roots list, substituting the default
// when none are configured.
func (r *Resolver) roots() []string {
	if len(r.Roots) == 0 {
		return []string{DefaultRoot}
	}
	return r.Roots
}

// EffectiveRoots returns the list of root directories actually
// searched, substituting the default when none are configured. Each
// returned path is joined with RootDir when RootDir is non-empty, so
// the result is suitable for user-facing diagnostics.
func (r *Resolver) EffectiveRoots() []string {
	base := r.roots()
	if r.RootDir == "" {
		return append([]string(nil), base...)
	}
	out := make([]string, len(base))
	for i, root := range base {
		out[i] = filepath.Join(r.RootDir, root)
	}
	return out
}

// readDir lists a root directory. When r.FS is explicitly set it is
// used directly. Otherwise Resolver falls back to raw os-level
// operations joined with r.RootDir, which permits ".." segments that
// os.DirFS rejects. This fallback is looser than RootFS-backed
// resolution — callers in the required-structure rule separately
// reject parent-traversal roots when RootFS is present, so the
// looseness only surfaces in fixture-style tests and CLI invocations
// where RootFS is not configured.
func (r *Resolver) readDir(root string) ([]fs.DirEntry, error) {
	if r.FS != nil {
		return fs.ReadDir(r.FS, root)
	}
	return os.ReadDir(r.osJoin(root))
}

func (r *Resolver) stat(path string) (fs.FileInfo, error) {
	if r.FS != nil {
		return fs.Stat(r.FS, path)
	}
	return os.Stat(r.osJoin(path))
}

func (r *Resolver) readFile(path string) ([]byte, error) {
	if r.FS != nil {
		return fs.ReadFile(r.FS, path)
	}
	return os.ReadFile(r.osJoin(path))
}

func (r *Resolver) osJoin(p string) string {
	if r.RootDir == "" {
		return p
	}
	return filepath.Join(r.RootDir, p)
}

// List returns every discovered archetype, sorted by name. When two
// roots contain an archetype with the same name, only the entry from
// the earlier root is returned. Files whose names do not qualify as
// archetype names (see isArchetypeName) are skipped — this keeps
// README.md, dotfiles, and underscore-prefixed scratch files out of
// the archetype namespace.
func (r *Resolver) List() []Entry {
	seen := map[string]bool{}
	var out []Entry
	for _, root := range r.roots() {
		cleanRoot := filepath.ToSlash(filepath.Clean(root))
		entries, err := r.readDir(cleanRoot)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			base := strings.TrimSuffix(name, ".md")
			if !isArchetypeName(base) {
				continue
			}
			if seen[base] {
				continue
			}
			seen[base] = true
			out = append(out, Entry{
				Name: base,
				Path: filepath.ToSlash(filepath.Join(cleanRoot, name)),
				Root: cleanRoot,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// isArchetypeName reports whether basename (without the ".md"
// extension) is a valid user-facing archetype name. Names starting
// with "_" or "." are reserved for scratch and hidden files. The
// case-insensitive names "readme", "license", and "contributing" are
// reserved for repository metadata so that files conventionally
// written into project directories do not accidentally surface as
// archetypes.
func isArchetypeName(base string) bool {
	if base == "" {
		return false
	}
	if strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") {
		return false
	}
	switch strings.ToLower(base) {
	case "readme", "license", "contributing", "codeowners":
		return false
	}
	return true
}

// Lookup returns the archetype with the given name. Missing names
// produce an error whose message names the searched roots. Reserved
// names (see isArchetypeName) produce an "unknown archetype" error
// even if the file exists on disk.
func (r *Resolver) Lookup(name string) (Entry, error) {
	if name == "" {
		return Entry{}, fmt.Errorf("archetype name must not be empty")
	}
	if !isArchetypeName(name) {
		return Entry{}, notFoundError(name, r.EffectiveRoots(), r.List())
	}
	for _, root := range r.roots() {
		cleanRoot := filepath.ToSlash(filepath.Clean(root))
		candidate := filepath.ToSlash(filepath.Join(cleanRoot, name+".md"))
		info, err := r.stat(candidate)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Entry{}, fmt.Errorf(
				"reading archetype %q: %w", name, err)
		}
		if info.IsDir() {
			continue
		}
		return Entry{Name: name, Path: candidate, Root: cleanRoot}, nil
	}
	return Entry{}, notFoundError(name, r.EffectiveRoots(), r.List())
}

// Content returns the raw bytes of the named archetype schema.
func (r *Resolver) Content(name string) ([]byte, error) {
	entry, err := r.Lookup(name)
	if err != nil {
		return nil, err
	}
	return r.readFile(entry.Path)
}

// AbsPath returns the filesystem path of the named archetype,
// joined with RootDir when RootDir is set.
func (r *Resolver) AbsPath(name string) (string, error) {
	entry, err := r.Lookup(name)
	if err != nil {
		return "", err
	}
	if r.RootDir == "" {
		return entry.Path, nil
	}
	return filepath.Join(r.RootDir, entry.Path), nil
}

func notFoundError(name string, roots []string, found []Entry) error {
	names := make([]string, len(found))
	for i, e := range found {
		names[i] = e.Name
	}
	rootList := strings.Join(roots, ", ")
	if len(names) == 0 {
		return fmt.Errorf(
			"unknown archetype %q: no archetypes found under roots [%s]",
			name, rootList)
	}
	return fmt.Errorf(
		"unknown archetype %q: available under roots [%s]: %s",
		name, rootList, strings.Join(names, ", "))
}
