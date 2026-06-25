// Package wordlist provides named, user-extensible word-lists. A
// word-list is an ordered set of literal strings with an optional
// `extends:` parent. The built-in lists — `ai-speak` (LLM vocabulary
// and phrase tells) and `ai-openers` (banned sentence openers) — ship
// embedded as data files; user lists live under
// `.mdsmith/wordlists/` and may extend a built-in or another user
// list. Rules consume resolved lists through the `lists:` setting; the
// config layer expands them, so this package depends on neither the
// rule nor the config package and stays free of import cycles.
package wordlist

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

// Wordlist is a named, ordered set of literal string entries with an
// optional Extends parent. SourcePath records the file a user list was
// loaded from, for provenance and error messages; it is empty for
// built-ins.
type Wordlist struct {
	Name       string
	Extends    string
	Entries    []string
	SourcePath string
}

// fileBody is the on-disk YAML shape: an optional `extends:` parent and
// the literal `entries:`. Strict decoding rejects any other key so a
// typo surfaces at config load.
type fileBody struct {
	Extends string   `yaml:"extends"`
	Entries []string `yaml:"entries"`
}

//go:embed data/ai-speak.yaml data/ai-openers.yaml
var builtinFS embed.FS

// builtins is the built-in list table, parsed once from the embedded
// data files at package init. A decode failure is a build-time
// contract violation (the data is checked in), so it panics.
var builtins = mustLoadBuiltins()

// Parse decodes a wordlist file body into its `extends:` parent and
// `entries:`. YAML anchors/aliases are rejected, and decoding is strict
// (an unknown top-level key is an error). An empty body is an error:
// a list with no entries cannot be referenced meaningfully.
func Parse(data []byte) (extends string, entries []string, err error) {
	if err := yamlutil.RejectYAMLAliases(data); err != nil {
		return "", nil, err
	}
	var body fileBody
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return "", nil, fmt.Errorf("empty wordlist")
		}
		return "", nil, err
	}
	return body.Extends, body.Entries, nil
}

func mustLoadBuiltins() map[string]Wordlist {
	names := []string{"ai-speak", "ai-openers"}
	m := make(map[string]Wordlist, len(names))
	for _, n := range names {
		data, err := builtinFS.ReadFile("data/" + n + ".yaml")
		if err != nil {
			panic(fmt.Sprintf("wordlist: reading embedded %q: %v", n, err))
		}
		ext, entries, err := Parse(data)
		if err != nil {
			panic(fmt.Sprintf("wordlist: parsing embedded %q: %v", n, err))
		}
		m[n] = Wordlist{Name: n, Extends: ext, Entries: entries}
	}
	return m
}

// Builtin returns the built-in word-list with the given name.
func Builtin(name string) (Wordlist, bool) {
	wl, ok := builtins[name]
	return wl, ok
}

// BuiltinNames returns the sorted built-in list names. Callers use it
// for reserved-name checks: a user file must not redefine a built-in.
func BuiltinNames() []string {
	names := make([]string, 0, len(builtins))
	for n := range builtins {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Lookup returns the word-list with the given name. User lists (the
// passed map, which may be nil) are checked first, then the built-ins.
// The error lists every valid name from both sets so a typo is easy to
// fix.
func Lookup(name string, user map[string]Wordlist) (Wordlist, error) {
	if wl, ok := user[name]; ok {
		return wl, nil
	}
	if wl, ok := builtins[name]; ok {
		return wl, nil
	}
	return Wordlist{}, fmt.Errorf(
		"unknown wordlist %q (valid: %s)", name, strings.Join(allNames(user), ", "),
	)
}

// allNames returns the sorted union of user and built-in list names.
func allNames(user map[string]Wordlist) []string {
	set := make(map[string]struct{}, len(user)+len(builtins))
	for n := range user {
		set[n] = struct{}{}
	}
	for n := range builtins {
		set[n] = struct{}{}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Resolve returns the fully-resolved entries for name: the `extends:`
// chain flattened parent-first, de-duplicated with the first
// occurrence winning. It errors on an unknown name, an unknown parent,
// or an `extends:` cycle.
func Resolve(name string, user map[string]Wordlist) ([]string, error) {
	raw, err := flatten(name, user, map[string]bool{})
	if err != nil {
		return nil, err
	}
	return dedup(raw), nil
}

// flatten walks the (single-parent) extends chain parent-first,
// accumulating entries. seen carries the names on the current chain so
// a cycle is caught; because each list has at most one parent the chain
// is linear, so a name removed from any path cannot reappear except via
// a cycle.
func flatten(name string, user map[string]Wordlist, seen map[string]bool) ([]string, error) {
	if seen[name] {
		return nil, fmt.Errorf("wordlist %q: extends cycle", name)
	}
	wl, err := Lookup(name, user)
	if err != nil {
		return nil, err
	}
	seen[name] = true
	var out []string
	if wl.Extends != "" {
		parent, err := flatten(wl.Extends, user, seen)
		if err != nil {
			return nil, fmt.Errorf("wordlist %q extends %w", name, err)
		}
		out = append(out, parent...)
	}
	out = append(out, wl.Entries...)
	return out, nil
}

// dedup returns ss with later duplicates removed, preserving the first
// occurrence's position. Returns nil for an empty input.
func dedup(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
