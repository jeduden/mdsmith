// Package archetypes ships built-in required-structure schemas
// for common agentic Markdown document types.
package archetypes

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.md
var files embed.FS

// Lookup returns the bytes of the built-in archetype schema with the
// given name (for example "story-file"). The name is the basename
// without extension. An unknown name returns an error whose message
// lists the available archetypes; other read errors are wrapped and
// returned as-is.
func Lookup(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("archetype name must not be empty")
	}
	data, err := files.ReadFile(name + ".md")
	if err != nil {
		return nil, classifyLookupError(name, err, List())
	}
	return data, nil
}

// classifyLookupError turns a ReadFile error into a user-facing
// error. Missing entries surface as "unknown archetype" with the
// available list; other errors are wrapped verbatim.
func classifyLookupError(name string, err error, available []string) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf(
			"unknown archetype %q: available: %s",
			name, strings.Join(available, ", "))
	}
	return fmt.Errorf("reading archetype %q: %w", name, err)
}

// List returns the names of all built-in archetypes, sorted.
func List() []string {
	entries, _ := files.ReadDir(".")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	return names
}
