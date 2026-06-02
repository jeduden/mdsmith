package schema

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// SchemaDiagnostic is the structured form of an MDS020 violation.
// Every CUE-error, structure-mismatch, filename-mismatch, and
// require-directive failure flows through this type so the
// rendered message names the field, shows the value, names the
// constraint, and (when applicable) suggests a fix.
//
// The type lives in the schema package because schema.Validate is
// the producer; internal/rules/requiredstructure is the sole
// consumer and would import this from anywhere else. Keeping the
// formatter alongside the producer avoids duplicating extractor
// logic across the two packages and sidesteps the import cycle
// that would result from defining it under internal/rules/.
//
// The `Schema` prefix on the name disambiguates from lint.Diagnostic
// at import sites where both packages are visible — plan 147 names
// the type explicitly, and renaming to schema.Diagnostic would
// require touching every caller for purely cosmetic reasons.
//
//nolint:revive // intentional name; see comment above.
type SchemaDiagnostic struct {
	// Field names the offending input. For front-matter CUE errors
	// it is the top-level key (or dotted path for nested values).
	// For structure failures it is the formatted heading
	// (e.g. "## Goal"). For filename failures it is "filename".
	Field string

	// Actual is the value the user wrote, rendered for display:
	// strings appear quoted, scalars raw. Empty when the user
	// supplied nothing concrete (e.g. a missing required section).
	Actual string

	// Expected describes the constraint in user vocabulary
	// (e.g. `one of: "open", "in-progress", "done"`). Empty when
	// the expectation is implied by Field — for instance, a
	// "missing required section" diagnostic does not repeat the
	// section name as the expected value.
	Expected string

	// Hint, when non-empty, points the reader at a likely fix
	// (typically the nearest valid literal or numeric bound).
	// Hints are best-effort; a noisy hint is worse than none, so
	// the extractor only fires on a small set of shapes.
	Hint string

	// SchemaRef names the schema source and (when known) the line
	// of the constraint, so the reader can locate the rule
	// without parsing the message. Examples: "plan/proto.md:4",
	// "inline kind schema".
	SchemaRef string

	// Deprecated marks a diagnostic raised because the named field
	// carries a `deprecated: true` flag in its schema. The renderer
	// uses the deprecated form ("deprecated field") instead of the
	// got/expected pair. Plan 136.
	Deprecated bool

	// ReplacedBy carries the schema's `replaced-by:` hint for a
	// deprecated field. When set and DeprecationMessage is empty,
	// Format() renders the canonical "replaced by `name`" sentence
	// so the diagnostic still points the reader at the new field.
	ReplacedBy string

	// DeprecationMessage carries the schema's `message:` payload for
	// a deprecated field. When non-empty it wins over ReplacedBy for
	// the human-facing line; ReplacedBy still rides on the
	// lint.Diagnostic so tooling can route the change without
	// parsing the message.
	DeprecationMessage string
}

// Format renders the human-facing message: the first line carries
// field/actual/expected, and an optional hint follows in parentheses
// on its own indented line. Per plan 230 the schema reference no
// longer rides on the message — Emit attaches it as a structured
// lint.RelatedLocation so the CLI and the LSP surface it as a
// navigable location rather than greppable text.
//
// When Deprecated is true the renderer switches to plan 136's
// deprecation shape: the first line reads
// "<field>: deprecated field" optionally followed by
// "; replaced by `<name>`" when ReplacedBy is set, then an
// optional message line.
func (d SchemaDiagnostic) Format() string {
	if d.Deprecated {
		return d.formatDeprecated()
	}
	var b strings.Builder
	b.WriteString(d.Field)
	if d.Actual != "" {
		b.WriteString(": got ")
		b.WriteString(d.Actual)
	}
	if d.Expected != "" {
		if d.Actual == "" {
			b.WriteString(": expected ")
		} else {
			b.WriteString(", expected ")
		}
		b.WriteString(d.Expected)
	}
	if d.Hint != "" {
		b.WriteString("\n  (")
		b.WriteString(d.Hint)
		b.WriteString(")")
	}
	return b.String()
}

// formatDeprecated renders the deprecation form. The field name
// leads the message so editor tooltips and CI log scans share the
// same anchor as the standard (got/expected) shape. `message:`
// wins over `replaced-by:` for the human-facing line per plan
// 136; the structured ReplacedBy still rides on the diagnostic so
// LSP clients can route on it.
func (d SchemaDiagnostic) formatDeprecated() string {
	var b strings.Builder
	b.WriteString(d.Field)
	b.WriteString(": deprecated field")
	if d.DeprecationMessage == "" && d.ReplacedBy != "" {
		b.WriteString("; replaced by `")
		b.WriteString(d.ReplacedBy)
		b.WriteString("`")
	}
	if d.DeprecationMessage != "" {
		b.WriteString("\n  message: ")
		b.WriteString(d.DeprecationMessage)
	}
	return b.String()
}

// Emit builds the lint.Diagnostic for this schema diagnostic. It runs
// the message through mk (which fills in file, line, rule ID, and
// source context) and then attaches the schema reference as a
// RelatedLocation so the CLI prints it as a trailer and the LSP maps
// it onto relatedInformation. The (file, line, msg) MakeDiag signature
// is unchanged across its call sites; Emit only augments the result.
func (d SchemaDiagnostic) Emit(mk MakeDiag, file string, line int) lint.Diagnostic {
	diag := mk(file, line, d.Format())
	if rl, ok := d.related(); ok {
		diag.RelatedLocations = append(diag.RelatedLocations, rl)
	}
	return diag
}

// related converts the textual SchemaRef into a structured
// RelatedLocation. A ref of the form "<path>:<line>" or "<path>"
// yields a navigable location; a descriptive label that names no file
// (e.g. "inline kind schema" or "kinds[task] / path-pattern") yields a
// message-only location so the reference still shows in CLI output
// while the LSP, which needs a URI, skips it. Returns ok=false when
// there is no reference at all.
func (d SchemaDiagnostic) related() (lint.RelatedLocation, bool) {
	if d.SchemaRef == "" {
		return lint.RelatedLocation{}, false
	}
	file, line := parseSchemaRef(d.SchemaRef)
	if file == "" {
		// Label-only ref: keep the label as the message; no file means
		// no navigable URI for the LSP, which drops empty-file entries.
		return lint.RelatedLocation{Message: d.SchemaRef}, true
	}
	return lint.RelatedLocation{File: file, Line: line, Message: d.relatedMessage()}, true
}

// relatedMessage is the short label shown on a navigable schema
// related-location. The detailed expectation stays in the main
// diagnostic message, so this stays concise.
func (d SchemaDiagnostic) relatedMessage() string {
	if d.Deprecated {
		return "deprecated by schema"
	}
	return "required by schema"
}

// parseSchemaRef splits a SchemaRef into a file path and 1-based line.
// It recognizes "<path>:<line>" when the suffix is a positive integer
// and the prefix looks like a path, and "<path>" on its own. Anything
// that is not a path (labels with spaces, single words with no
// separator) returns ("", 0) so the caller treats it as a label.
func parseSchemaRef(ref string) (file string, line int) {
	if i := strings.LastIndexByte(ref, ':'); i > 0 {
		if n, err := strconv.Atoi(ref[i+1:]); err == nil && n > 0 {
			if cand := ref[:i]; looksLikePath(cand) {
				return cand, n
			}
		}
	}
	if looksLikePath(ref) {
		return ref, 0
	}
	return "", 0
}

// looksLikePath reports whether s is plausibly a repo-relative file
// path: no whitespace, and at least one "/" or "." separator. Schema
// sources are POSIX paths like "plan/proto.md", so this rejects the
// descriptive labels ("inline kind schema", "schema") without a false
// positive on a real path.
func looksLikePath(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	return strings.ContainsAny(s, "/.")
}

// formatActual renders a JSON-decoded front-matter value for the
// "got" segment of the diagnostic. Strings are quoted with %q so
// the rendered message is unambiguous; numbers and bools pass
// through Sprintf's default formatting. An explicit YAML/JSON
// null surfaces as the literal `null` so callers can distinguish
// "field present but null" from "field absent" — the latter sets
// Actual to "<missing>" at the call site (see lookupFM's
// hasActual contract). Complex shapes (maps, slices) round-trip
// through json.Marshal so the rendered output is deterministic
// (Go's default map formatting iterates keys in undefined order,
// which would make the diagnostic message non-stable and break
// the deduplication key the caller derives from Actual).
func formatActual(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("%q", x)
	case bool, int, int64, float64:
		return fmt.Sprintf("%v", x)
	}
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}
