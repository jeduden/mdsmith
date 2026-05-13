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

	// CrossReferences lists patterns whose matches in the document
	// body must resolve to a heading slug. See plan 143.
	CrossReferences []CrossRef

	// Acronyms, if non-nil, asks the validator to flag first-use
	// acronyms (length 2-6, all caps) that lack a parenthesised
	// expansion. See plan 143.
	Acronyms *AcronymRule

	// Index, if non-nil, asks `mdsmith fix` to emit a JSON
	// side-output describing the document. `mdsmith check` reports
	// staleness (missing or outdated file) as a diagnostic so the
	// fixer is triggered, but never writes the file itself. See
	// plan 143.
	Index *IndexSpec
}

// CrossRef declares one text-pattern → slug-template binding. The
// validator searches text nodes for Pattern; for each match it fills
// the captured groups (numeric `{n}` or named `{slug}`) into
// MustMatch, slugifies the result, and looks it up in the document's
// heading slug set. Lines whose raw text matches SkipLinesMatching
// are exempt — typically blockquoted stale text.
type CrossRef struct {
	Pattern           string
	MustMatch         string
	SkipLinesMatching string
}

// AcronymRule configures first-use acronym detection. KnownSafe is
// the allowlist of tokens that may appear without expansion. Scope,
// if non-empty, restricts the check to text inside sections whose
// heading text matches one of the listed names; empty Scope applies
// the check document-wide.
type AcronymRule struct {
	KnownSafe []string
	Scope     []string
}

// IndexSpec configures the index side-output. Output is the path
// (relative to the source file's directory) where `mdsmith fix`
// writes the JSON document. Include selects which sub-objects are
// emitted; the set is closed so downstream tools can parse the file
// without referencing a schema.
type IndexSpec struct {
	Output  string
	Include []string
}

// Valid include keys for IndexSpec.Include.
const (
	IndexIncludeStepMap      = "step-map"
	IndexIncludeCrossRefs    = "cross-ref-graph"
	IndexIncludeWordCounts   = "word-counts"
	IndexIncludeHeadingsFlat = "headings"
)

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
	// the document. Literal scopes default to true. Slot scopes
	// (`heading: {unlisted: true}`) always parse to Required=false
	// because the parser rejects an explicit `required:` key on
	// them. Preamble scopes (`heading: null`) default to false but
	// accept an explicit `required:` value; the inline validator
	// does not yet act on it (a future plan that adds preamble-
	// content checks will).
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

	// Content carries non-heading AST-node constraints inside the
	// matched section: required code blocks, tables, lists, and
	// paragraphs, in positional order. Plan 149 added this; entries
	// follow the same out-of-order + unlisted-slot semantics the
	// heading-tree validator uses.
	Content []ContentEntry
}

// Content-entry kind discriminators. The on-disk YAML carries one of
// these strings under `kind:`; the validator dispatches on the same
// constant.
const (
	ContentKindCodeBlock = "code-block"
	ContentKindTable     = "table"
	ContentKindList      = "list"
	ContentKindParagraph = "paragraph"
	ContentKindUnlisted  = "unlisted"
)

// ContentEntry describes one positional non-heading AST node that
// must appear inside a section's body. Each entry has a discriminator
// (Kind) and a small set of kind-specific constraint fields. Fields
// not relevant to the entry's kind are zero-valued.
type ContentEntry struct {
	// Kind names the AST shape this entry matches. One of
	// `ContentKind*`. Reject unknown values at parse time.
	Kind string

	// Required reports whether a matching node must appear at this
	// position. Defaults to true for literal entries; the parser
	// rejects `required:` on `kind: unlisted` outright.
	Required bool

	// Lang constrains a `code-block` entry's info string. Empty means
	// any language is accepted. Today the match is exact equality; a
	// future plan can extend the field with a regex form.
	Lang string

	// Columns constrains a `table` entry's header row. Empty means any
	// header is accepted. When set, the doc table's header row must
	// equal Columns element-wise.
	Columns []string

	// Ordered, OrderedSet constrain a `list` entry's bullet style.
	// OrderedSet reports whether the schema author set `ordered:`;
	// when false, any bullet style passes.
	Ordered    bool
	OrderedSet bool

	// MinItems and MaxItems bound the count of items in a `list`
	// entry's match. Zero means unbounded.
	MinItems int
	MaxItems int
}

// IsEmpty reports whether s carries no constraints. Used by callers
// (notably MDS020) to short-circuit when a kind declares no schema.
// A schema that declares only `content:` somewhere in its scope tree
// is not empty — anyScopeHasContent traverses Sections for it.
func (s *Schema) IsEmpty() bool {
	if s == nil {
		return true
	}
	return len(s.Frontmatter) == 0 &&
		s.Require.Filename == "" &&
		len(s.Sections) == 0 &&
		len(s.CrossReferences) == 0 &&
		s.Acronyms == nil &&
		s.Index == nil
}

// EffectiveRootLevel returns the heading level of the root scope
// list, falling back to 2 when unset.
func (s *Schema) EffectiveRootLevel() int {
	if s == nil || s.RootLevel <= 0 {
		return 2
	}
	return s.RootLevel
}
