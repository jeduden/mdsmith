// Package pack defines additive init scaffolds: named bundles of
// `.mdsmith/` sidecar files that `mdsmith init --add <name>` writes
// beside `.mdsmith.yml` without clobbering existing files.
//
// A pack is additive and idempotent by contract. Applying it never
// overwrites a file that is already present, so re-running init or
// layering a pack onto an already-configured project is safe. Packs are
// the extension axis for curated `.mdsmith/` content — word-lists today,
// kind packs and stopword lists on the roadmap. Each new bundle
// registers here as data rather than growing a bespoke init flag, which
// keeps `mdsmith init --add` the single mechanism for every additive
// scaffold.
//
// Naming note: a pack is unrelated to a starter. A starter (package
// starter) replaces the whole `.mdsmith.yml`; a pack only adds sidecar
// files under `.mdsmith/` and leaves the config untouched.
package pack

import (
	"fmt"
	"sort"
	"strings"
)

// File is one sidecar file a pack writes: a workspace-relative path and
// the bytes to put there.
type File struct {
	Path string
	Data []byte
}

// Pack is a named additive scaffold. Summary is the one-line description
// shown by `mdsmith init --list`; files renders the bundle's contents on
// demand, so a pack that draws on another package's data stays lazy.
type Pack struct {
	Name    string
	Summary string
	files   func() []File
}

// Files renders the pack's sidecar files.
func (p Pack) Files() []File { return p.files() }

// registry holds every pack, keyed by name. Packs register themselves
// from their own file's init(), mirroring how rules populate their
// registry.
var registry = map[string]Pack{}

func register(p Pack) { registry[p.Name] = p }

// Get returns the pack with the given name. ok is false when no pack has
// that name.
func Get(name string) (p Pack, ok bool) {
	p, ok = registry[name]
	return p, ok
}

// Names returns the registered pack names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// All returns every registered pack, sorted by name.
func All() []Pack {
	names := Names()
	out := make([]Pack, 0, len(names))
	for _, name := range names {
		out = append(out, registry[name])
	}
	return out
}

// ErrUnknown formats an "unknown pack" error that lists every valid
// name, mirroring starter.ErrUnknown.
func ErrUnknown(name string) error {
	return fmt.Errorf("unknown pack %q (valid: %s)", name, strings.Join(Names(), ", "))
}
