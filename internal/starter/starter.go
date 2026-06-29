// Package starter serves ready-to-use `.mdsmith.yml` configurations for
// a specific authoring workflow. `mdsmith init --starter <name>` writes
// the named starter instead of the rule-by-rule defaults dump. Each
// starter is a hand-authored, commented template embedded from
// templates/, so the file a user lands on explains itself and is meant
// to be edited.
//
// Naming note: this is deliberately not called a "recipe" — that term
// belongs to the `<?build?>` directive's command (and MDS040
// recipe-safety). A starter is a starting configuration, nothing to do
// with the build system.
package starter

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed templates/*.yml
var templatesFS embed.FS

// Get returns the `.mdsmith.yml` bytes for the named starter. ok is
// false when no starter has that name.
func Get(name string) (data []byte, ok bool) {
	b, err := templatesFS.ReadFile("templates/" + name + ".yml")
	if err != nil {
		return nil, false
	}
	return b, true
}

// Names returns the available starter names, sorted.
func Names() []string { return namesFrom(templatesFS) }

func namesFrom(fsys fs.FS) []string {
	entries, err := fs.ReadDir(fsys, "templates")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yml"))
	}
	sort.Strings(names)
	return names
}

// ErrUnknown formats an "unknown starter" error that lists every valid
// name, mirroring how convention.Lookup reports an unknown convention.
func ErrUnknown(name string) error {
	return fmt.Errorf(
		"unknown starter %q (valid: %s)", name, strings.Join(Names(), ", "))
}
