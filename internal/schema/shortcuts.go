package schema

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	cuetypes "github.com/jeduden/mdsmith/cue/types"
)

// EmbeddedTypesCUE returns the source of the shortcut
// library shipped at `cue/types/types.cue`. The
// runtime registry below is the actual lookup table;
// this accessor exposes the documented source for
// tooling (e.g. an `mdsmith help schema-types`
// command) and for the drift test that pins registry
// entries to the file contents.
func EmbeddedTypesCUE() string { return cuetypes.Source() }

// shortcutRegistry maps each registered short name to
// the canonical CUE expression substituted in its place
// when a schema's `frontmatter:` value is the bare name.
// The keys mirror the `#name` definitions in
// `cue/types/types.cue`; the values are the right-hand
// side of each definition verbatim. A test enforces the
// match so the two stay in sync.
var shortcutRegistry = map[string]string{
	"date":     `=~"^\\d{4}-\\d{2}-\\d{2}$"`,
	"datetime": `=~"^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(Z|[+-]\\d{2}:\\d{2})?$"`,
	"time":     `=~"^\\d{2}:\\d{2}(:\\d{2})?$"`,
	"email":    `=~"^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$"`,
	"url":      `=~"^https?://"`,
	"filename": `=~"^[A-Za-z0-9._-]+\\.md$"`,
	"nonEmpty": `string & !=""`,
}

// cueBuiltinTypes lists the bare identifiers CUE accepts
// as built-in types (and the `_` top value). They look
// like shortcut candidates by spelling, but they
// already resolve in raw CUE, so the parser must let
// them pass through unchanged. Without this carve-out,
// every `bool` or `string` frontmatter value in the
// existing proto.md fixtures would error as an
// "unknown shortcut".
var cueBuiltinTypes = map[string]struct{}{
	"null":   {},
	"bool":   {},
	"int":    {},
	"float":  {},
	"string": {},
	"bytes":  {},
	"number": {},
	"_":      {},
}

// bareNamePattern recognises a YAML scalar value that
// might be a shortcut name. The shape is deliberately
// looser than a strict CUE identifier so a hyphenated
// typo like `iso-date` is still caught at parse time
// and surfaces as a clear error rather than sliding
// through to the CUE compiler as an undefined
// reference.
var bareNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)

// LookupShortcut returns the canonical CUE expression
// registered under name, or (` `, false) when name is
// not in the registry. Callers should use
// resolveBareName for end-to-end shortcut handling;
// this helper exists for tests and for tools that
// want to introspect a single entry.
func LookupShortcut(name string) (string, bool) {
	v, ok := shortcutRegistry[name]
	return v, ok
}

// ShortcutNames returns the sorted list of registered
// shortcut names. Used by error messages, the docs
// page, and the drift test.
func ShortcutNames() []string {
	out := make([]string, 0, len(shortcutRegistry))
	for k := range shortcutRegistry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolveBareName classifies the trimmed YAML string s
// and dispatches it through the shortcut registry when
// applicable. The three outcomes:
//
//   - (rewritten, true, nil) — s is a registered
//     shortcut name; rewritten is the canonical CUE
//     expression to substitute.
//   - (s, true, nil) — s is a CUE built-in type
//     (`string`, `bool`, …); pass through verbatim.
//   - (s, false, nil) — s is not a bare-name candidate
//     (contains operators, whitespace, quotes, …);
//     caller passes it through as raw CUE.
//   - ("", true, err) — s looks like a shortcut name
//     but is not registered; the error names the
//     unknown bare name and lists the known shortcuts.
//
// The "handled" boolean lets the caller distinguish
// "we deliberately picked this up" from "we didn't
// touch it" so a future audit can tell whether a
// regression in shortcut logic could have changed a
// value's interpretation.
func resolveBareName(s string) (string, bool, error) {
	if !bareNamePattern.MatchString(s) {
		return s, false, nil
	}
	if _, ok := cueBuiltinTypes[s]; ok {
		return s, true, nil
	}
	if v, ok := shortcutRegistry[s]; ok {
		return v, true, nil
	}
	return "", true, fmt.Errorf(
		"unknown shortcut %q (known: %s); use a quoted CUE expression "+
			"for raw CUE, or check spelling — see "+
			"docs/reference/schema-types.md",
		s, strings.Join(ShortcutNames(), ", "))
}
