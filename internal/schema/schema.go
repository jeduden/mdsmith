// Package schema models the document-structure schemas that drive
// MDS020 (required-structure). A schema describes what a Markdown
// document's front matter, filename, and heading tree must look like.
//
// Two sources feed the same in-memory representation:
//
//   - Inline. A YAML schema block under kinds.<name>.schema: in
//     .mdsmith.yml.
//   - File. A proto.md file referenced by
//     rules.required-structure.schema: (the legacy heading-template
//     form).
//
// Both parse into a Schema whose Sections is a recursive tree of
// Scope nodes. The validator walks a document's AST against that
// tree, emitting diagnostics through the lint.Diagnostic shape.
//
// See plan/146_inline-schema-in-kinds.md for the design context.
package schema

// SectionWildcard is the literal text the file-based parser
// recognises in a proto.md heading row (`## ...`) as a positional
// slot — the on-disk surface for what the inline grammar spells
// `heading: {unlisted: true}`. The inline parser rejects this
// string when it appears as `heading:` or `aliases:` text;
// authors must use the mapping form. The constant lives here so
// the two parsers agree on the same on-disk token.
const SectionWildcard = "..."

// Schema is the parsed representation of a single document schema.
// It is produced by the inline YAML parser or the proto.md file
// parser; both feed the same struct.
type Schema struct {
	// Frontmatter maps each front-matter key to a CUE expression that
	// constrains its value. The map preserves user keys verbatim,
	// including any trailing "?" optional-field marker.
	Frontmatter map[string]string

	// Require carries constraints that apply to the document as a
	// whole (filename pattern, etc.).
	Require Require

	// Sections holds the top-level section list at RootLevel; each
	// Scope may itself nest further sections one heading level
	// deeper. Inline schemas always start at H2 (RootLevel=2), so
	// the document H1 is owned by first-line-heading and any
	// title-bearing frontmatter field rather than represented here.
	// File-based schemas can root at H1 (e.g. a `# ?` wildcard in
	// proto.md, RootLevel=1) — that H1 scope is part of Sections
	// and its children appear at level 2.
	Sections []Scope

	// Closed reports whether the root scope is strict: when true,
	// unlisted top-level headings produce a diagnostic; when false,
	// they are tolerated between listed sections. File-based schemas
	// default to Closed=true to preserve the historical
	// heading-template semantics; inline schemas default to false
	// per plan 146.
	Closed bool

	// Source is a human-readable label (file path for file-based
	// schemas, kind name for inline schemas) used in diagnostics
	// referring to the schema itself.
	Source string

	// RootLevel is the heading level of entries in Sections.
	// Inline schemas use 2 (H1 belongs to the title). File-based
	// schemas adopt whatever level the topmost heading in the
	// proto.md uses — usually 1 for a `# ?` wildcard, 2 when the
	// file declares only `## ...` rows.
	RootLevel int
}

// Require captures the schema-level constraints that apply to the
// document as a whole.
type Require struct {
	// Filename is a glob the document basename must match. Empty
	// means no filename constraint.
	Filename string
}

// Scope binds an AST subtree (a section) to a set of constraints and
// per-rule config overrides. The root scope's children are the
// top-level (H2) section list; their children are H3, and so on.
// Levels come from depth in the tree.
type Scope struct {
	// Heading is the heading text to match. No "#" markers; the
	// level comes from depth in the tree. The single-character
	// "?" matches any text. Headings (and aliases) containing
	// `{field}` interpolation are matched as anchored regex
	// patterns, with each placeholder consuming one or more
	// characters of the doc heading text. Empty when Wildcard is
	// true.
	Heading string

	// Required reports whether a matching heading must appear in
	// the document. Literal scopes default to true; preamble and
	// slot (wildcard) scopes have no heading to require — both
	// parsers leave Required at false for them. The parser rejects
	// an explicit `required:` key on preamble or slot entries so
	// the in-memory value is unambiguous.
	Required bool

	// Aliases lists alternate heading texts that match this scope.
	// An empty list means only Heading matches.
	Aliases []string

	// Sections is the recursive list of nested sections (one level
	// deeper in the document tree).
	Sections []Scope

	// Repeats reports whether Heading is a pattern (with placeholder
	// tokens) that may match zero or more sections.
	Repeats bool

	// Sequential, on a repeating scope, asserts no gaps and no
	// duplicates in the {n} placeholder values.
	Sequential bool

	// Min and Max bound the match count of a repeating scope. Zero
	// means unbounded.
	Min int
	Max int

	// Closed reports whether this scope is strict: when true,
	// unlisted child headings produce a diagnostic; when false, they
	// are tolerated between listed sub-sections.
	Closed bool

	// Wildcard reports whether this scope is a slot that matches
	// zero or more sections the schema did not list by name. Authors
	// write it inline as `heading: {unlisted: true}` (or as a `## ...`
	// row in a file-based proto.md). Out-of-order detection still
	// claims a heading whose text matches a later listed scope, so
	// the slot only absorbs truly-unlisted sections.
	Wildcard bool

	// Preamble reports whether this scope describes the implicit
	// section before any heading — the document's lead-in content.
	// Authors write it inline as `heading: null`. A preamble scope
	// has no heading text to match; its range is [parent-start,
	// first-child-heading). Plan 146 limits the preamble to
	// carrying `rules:` overrides for that range; `content:` (plan
	// 149) extends it to AST-node constraints.
	Preamble bool

	// Rules carries per-scope rule-config overrides. Each entry maps
	// a rule name to a settings map. The MDS020 walker re-runs each
	// named rule with these settings against the document and
	// filters diagnostics to the scope's heading range.
	//
	// Today the override stacks on the rule's defaults, not the
	// file's full effective config (defaults → kinds → file globs →
	// scope). Threading the full merge through the engine is a
	// tracked follow-up on plan 146; see docs/guides/schemas.md.
	Rules map[string]map[string]any
}

// IsEmpty reports whether s carries no constraints. Used by callers
// (notably MDS020) to short-circuit when a kind declares no schema.
func (s *Schema) IsEmpty() bool {
	if s == nil {
		return true
	}
	return len(s.Frontmatter) == 0 &&
		s.Require.Filename == "" &&
		len(s.Sections) == 0
}

// EffectiveRootLevel returns the heading level of the root scope
// list, falling back to 2 when unset.
func (s *Schema) EffectiveRootLevel() int {
	if s == nil || s.RootLevel <= 0 {
		return 2
	}
	return s.RootLevel
}
