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

func (r *Resolver) fs() fs.FS {
	if r.FS != nil {
		return r.FS
	}
	dir := r.RootDir
	if dir == "" {
		dir = "."
	}
	return os.DirFS(dir)
}

// List returns every discovered archetype, sorted by name. When two
// roots contain an archetype with the same name, only the entry from
// the earlier root is returned.
func (r *Resolver) List() []Entry {
	seen := map[string]bool{}
	var out []Entry
	fsys := r.fs()
	for _, root := range r.roots() {
		cleanRoot := filepath.ToSlash(filepath.Clean(root))
		entries, err := fs.ReadDir(fsys, cleanRoot)
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

// Lookup returns the archetype with the given name. Missing names
// produce an error whose message names the searched roots.
func (r *Resolver) Lookup(name string) (Entry, error) {
	if name == "" {
		return Entry{}, fmt.Errorf("archetype name must not be empty")
	}
	fsys := r.fs()
	for _, root := range r.roots() {
		cleanRoot := filepath.ToSlash(filepath.Clean(root))
		candidate := filepath.ToSlash(filepath.Join(cleanRoot, name+".md"))
		info, err := fs.Stat(fsys, candidate)
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
	return Entry{}, notFoundError(name, r.roots(), r.List())
}

// Content returns the raw bytes of the named archetype schema.
func (r *Resolver) Content(name string) ([]byte, error) {
	entry, err := r.Lookup(name)
	if err != nil {
		return nil, err
	}
	return fs.ReadFile(r.fs(), entry.Path)
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
