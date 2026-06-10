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
// Plan 2606022124 collapses each section entry to one discriminator
// (`heading:` — null, string, or mapping) with a single matcher
// (`regex:`) and a cardinality field (`repeat: {min, max}`); see
// docs/reference/section-schema.md for the full grammar.
package schema

import "regexp"

// SectionWildcard is the literal text the file-based parser
// recognises in a proto.md heading row (`## ...`) as a positional
// slot — a heading run that matches any text zero or more times.
// The inline parser rejects this string when it appears as
// `heading:` text; authors must use the mapping form
// (`heading: { regex: '.+', repeat: { min: 0 } }`).
const SectionWildcard = "..."

// Schema is the parsed representation of a single document schema.
// It is produced by the inline YAML parser or the proto.md file
// parser; both feed the same struct.
type Schema struct {
	// Frontmatter maps each front-matter key to a CUE expression that
	// constrains its value. The map preserves user keys verbatim,
	// including any trailing "?" optional-field marker.
	Frontmatter map[string]string

	// FrontmatterLines maps each front-matter key to the 1-based
	// line of its constraint in the schema source, when known. The
	// file-based parser populates this from the proto.md
	// frontmatter via yaml.Node line metadata; the inline parser
	// does not (the config loader unmarshals into typed values
	// before this code sees it). Lookup is by the same key form
	// stored in Frontmatter (with the optional "?" suffix
	// preserved).
	FrontmatterLines map[string]int

	// FrontmatterMeta carries optional per-field metadata that
	// accompanies a constraint — today the deprecation flag and
	// its `message:` / `replaced-by:` hints (plan 136). The map
	// uses the same key form as Frontmatter (including any
	// trailing "?" optional-field marker) so lookups stay aligned
	// across the two maps. A field that omits metadata has no
	// entry here; the map is nil when no field declares any
	// metadata.
	FrontmatterMeta map[string]FieldMeta

	// Filename is a glob the document basename must match. Empty
	// means no filename constraint. Authors set it as a top-level
	// `filename:` key on the schema (plan 156 dropped the
	// `require.filename:` nesting).
	Filename string

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

// FieldMeta carries optional schema-side metadata for a single
// frontmatter field. The fields below populate plan 136's
// deprecation diagnostic: when Deprecated is true, the validator
// emits a Warning-severity SchemaDiagnostic whenever the field
// appears in a document's front matter. Message and ReplacedBy
// drive the wording — `message:` wins for the human-facing line,
// while `replaced-by:` is exposed in the structured diagnostic so
// LSP clients and CI tools can route the change without parsing
// the message body.
type FieldMeta struct {
	Deprecated bool
	Message    string
	ReplacedBy string
}

// CrossRef declares one text-pattern → slug-template binding. The
// validator searches text nodes for Pattern; for each match it fills
// the captured groups (numeric `{n}` or named `{slug}`) into
// MustMatch, slugifies the result, and looks it up in the document's
// heading slug set. Lines whose raw text matches SkipLinesMatching
// are exempt — typically blockquoted stale text.
//
// compiled and compiledSkip are set by parseCrossRefEntry at parse
// time so ValidateCrossReferences and buildCrossRefGraph never
// recompile the same NFA on every document. They are nil for CrossRef
// values constructed directly (e.g., in tests); compilePattern and
// compileSkip fall back to regexp.Compile in that case.
type CrossRef struct {
	Pattern           string
	MustMatch         string
	SkipLinesMatching string
	compiled          *regexp.Regexp // compiled Pattern; nil for test-constructed values
	compiledSkip      *regexp.Regexp // compiled SkipLinesMatching; nil if empty or test-constructed
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

// Scope binds an AST subtree (a section) to a set of constraints and
// per-rule config overrides. The root scope's children are the
// top-level (H2) section list; their children are H3, and so on.
// Levels come from depth in the tree.
//
// Plan 2606022124 collapses the section-entry vocabulary to three
// orthogonal axes:
//
//   - Discriminator — `heading:` value (`null`, string, or mapping).
//   - Matcher — `regex:` inside the mapping form.
//   - Cardinality — `repeat: { min, max }` inside the mapping form.
//
// The legacy `aliases:`, `required:`, scope-level
// `repeats:`/`sequential:`/`min:`/`max:`, and `{unlisted: true}`
// shapes are gone; the parser rejects them with a "removed; see
// plan 2606022124" diagnostic naming the replacement.
type Scope struct {
	// Heading is the diagnostic-friendly label for this scope —
	// the bare-string literal for the sugar form, the regex body
	// for the mapping form, or empty for the preamble. The
	// validator does not match on this field; Matcher drives all
	// heading claims.
	Heading string

	// Matcher, when non-nil, describes how this scope claims one or
	// more headings. Preamble scopes leave Matcher nil because they
	// have no heading text to compare against — their range is
	// `[parent-start, first-child-heading)`.
	Matcher *Matcher

	// Sections is the recursive list of nested sections (one level
	// deeper in the document tree).
	Sections []Scope

	// Preamble reports whether this scope describes the implicit
	// section before any heading — the document's lead-in content.
	// Authors write it inline as `heading: null`. A preamble scope
	// has no heading text to match; its range is [parent-start,
	// first-child-heading). Only valid as the first entry of its
	// section list.
	Preamble bool

	// Closed reports whether this scope is strict: when true,
	// unlisted child headings produce a diagnostic; when false,
	// they are tolerated between listed sub-sections.
	Closed bool

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

	// Bind, when non-nil, overrides the default schema-derived key
	// for this scope when projected by `mdsmith extract`. A non-empty
	// value renames the key; an explicit empty string (`bind: ""`)
	// hoists the scope's children into the parent instead of nesting
	// under a key. Nil means "use the default key". Plan 167.
	Bind *string
}

// Matcher describes how a Scope claims one or more consecutive
// headings. Authors set it inline as the mapping form
// (`heading: { regex, repeat?, sequential? }`); the bare-string
// sugar (`heading: "Overview"`) and the proto.md heading-row
// tokens (`## ?`, `## ...`, `## Step {n}`, `## {id}`) desugar to
// the same shape.
type Matcher struct {
	// Regex is the body of a CUE raw-interpolation string
	// (`#"..."#`). The validator compiles it as Go RE2 after
	// substituting the two helpers in scope: `digits` (the literal
	// string `(?P<n>[0-9]+)`) and `fmvar(name)` (the front-matter
	// field `name`, regex-escaped). Backslash passes through to RE2
	// without doubling; interpolation uses `\#(expr)` inside the
	// pattern.
	Regex string

	// Repeat bounds how many consecutive matching headings this
	// matcher claims. The zero value (Repeat{Set: false}) means
	// "exactly one"; see Repeat.Bounds for the canonical (min, max)
	// pair the validator consumes.
	Repeat Repeat

	// Sequential, with a `digits` named capture in Regex, asserts
	// the captured `n` values are strictly increasing without gaps.
	// Without a `digits` capture in the pattern the parser rejects
	// `sequential: true`.
	Sequential bool
}

// Repeat bounds the cardinality of a Matcher's heading run.
type Repeat struct {
	// Set reports whether the `repeat:` key was present in the YAML
	// mapping. When false, Bounds() returns (1, 1) — the matcher
	// claims exactly one heading. When true, an omitted `min:`
	// defaults to 0 and an omitted `max:` to unbounded (Max == 0).
	Set bool

	// Min is the minimum match count. Ignored when Set is false.
	Min int

	// Max is the maximum match count, with 0 meaning unbounded.
	// Ignored when Set is false. The parser rejects `repeat: {
	// max: 0 }` so Max == 0 unambiguously signals "unbounded".
	Max int
}

// Bounds returns the canonical (min, max) pair the validator
// consumes. Max == 0 in the returned pair means unbounded. A
// matcher with Set=false claims exactly one heading.
func (r Repeat) Bounds() (int, int) {
	if !r.Set {
		return 1, 1
	}
	return r.Min, r.Max
}

// Optional reports whether the matcher's run can be empty
// (min == 0). The validator treats Optional matchers as
// non-required: a missing match does not flag a diagnostic.
func (r Repeat) Optional() bool {
	min, _ := r.Bounds()
	return min == 0
}

// Required reports whether the scope claims at least one heading
// (Repeat.Min >= 1). Preamble scopes are never Required.
func (sc Scope) Required() bool {
	if sc.Preamble || sc.Matcher == nil {
		return false
	}
	return !sc.Matcher.Repeat.Optional()
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

	// Bind, when non-nil, overrides the default key for this content
	// entry when projected by `mdsmith extract` (`code` / `items` /
	// `rows` / `text`). A non-empty value renames the key; the empty
	// form (`bind: ""`) is rejected at parse time because a content
	// entry has no children to hoist into the parent. Nil means
	// "use the default key". Plan 167.
	Bind *string

	// Projection selects how `mdsmith extract` renders the matched
	// node. Empty means the kind's implicit default (`text` for a
	// paragraph, `code` for a code-block). One of `Projection*`.
	// `ProjectionInline` emits a paragraph's inline spans as a typed
	// recursive list and is rejected at parse time on any non-paragraph
	// kind; it also defaults the paragraph's extract key to `inline`
	// instead of `text` (unless `bind:` overrides it). Plan 212.
	Projection string
}

// Content-projection mode discriminators. The on-disk YAML carries
// one of these strings under a content entry's `projection:` key.
// An empty Projection means the kind's implicit default.
const (
	// ProjectionText emits a paragraph's plain text (the default for
	// `kind: paragraph`).
	ProjectionText = "text"
	// ProjectionCode emits a code-block's raw body (the default for
	// `kind: code-block`).
	ProjectionCode = "code"
	// ProjectionInline emits a paragraph's inline spans as a typed
	// recursive list (text / emphasis / strong / code / link /
	// autolink). Valid only on `kind: paragraph`.
	ProjectionInline = "inline"
	// ProjectionTree emits a list's items as a typed recursive tree:
	// each item is an object carrying its own `text`, an optional
	// `checked` bool for GFM task items, and optional `children` for a
	// nested sub-list. Valid only on `kind: list`. The flat default
	// (projection omitted) keeps emitting plain strings. Plan 244.
	ProjectionTree = "tree"
	// ProjectionRecords emits a table as an array of objects keyed by
	// column header — the default for `kind: table`. Duplicate column
	// headers cause an extract-time error because two cells would collide
	// on the same key. Plan 245.
	ProjectionRecords = "records"
	// ProjectionRows emits a table as a `{"columns": [...], "rows":
	// [[...]...]}` object — column order is preserved, duplicate headers
	// are accepted (the columns array is positional). Short body rows are
	// padded with empty strings to match the header width. Plan 245.
	ProjectionRows = "rows"
)

// IsEmpty reports whether s carries no constraints. Used by callers
// (notably MDS020) to short-circuit when a kind declares no schema.
// A schema that declares only `content:` somewhere in its scope tree
// is not empty — anyScopeHasContent traverses Sections for it.
func (s *Schema) IsEmpty() bool {
	if s == nil {
		return true
	}
	return len(s.Frontmatter) == 0 &&
		s.Filename == "" &&
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
