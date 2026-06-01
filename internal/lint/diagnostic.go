package lint

// Severity indicates the severity level of a diagnostic.
type Severity string

// Severity levels.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// LineRange is an inclusive 1-based line range within a source file.
type LineRange struct {
	From int
	To   int
}

// Contains reports whether the 1-based line l falls within r.
func (r LineRange) Contains(l int) bool {
	return l >= r.From && l <= r.To
}

// Diagnostic represents a single lint finding.
type Diagnostic struct {
	File            string
	Line            int
	Column          int
	RuleID          string
	RuleName        string
	Severity        Severity
	Message         string
	SourceLines     []string // context lines around the diagnostic; empty if unavailable
	SourceStartLine int      // 1-based line number of first entry in SourceLines
	// Explanation, when non-nil, attaches per-leaf provenance for the
	// rule that fired. Populated by the CLI when --explain is on.
	Explanation *Explanation

	// Deprecated reports whether the diagnostic flags a schema field
	// that has been marked deprecated. LSP clients and CI scripts read
	// this flag to route the diagnostic without parsing the message
	// text. Populated by MDS020 when a deprecated frontmatter field is
	// present in a document; zero on every other diagnostic.
	Deprecated bool

	// ReplacedBy carries the schema's `replaced-by:` hint when the
	// deprecation declares one. Empty on non-deprecation diagnostics
	// and on deprecation diagnostics that only set `message:`.
	ReplacedBy string

	// RelatedLocations carries secondary source locations that explain
	// the diagnostic — for MDS020, the schema constraint a value
	// violated. A location may point at a different file than the one
	// the diagnostic anchors in (the schema lives in proto.md, a kind
	// file, or .mdsmith.yml). The CLI prints these as trailer lines;
	// the LSP maps them onto DiagnosticRelatedInformation. Nil on
	// diagnostics that carry no secondary location.
	RelatedLocations []RelatedLocation

	// DocURL, when non-empty, is the canonical documentation URL for the
	// rule that fired. The LSP maps it onto Diagnostic.codeDescription
	// so the rule code renders as a clickable link; the CLI ignores it.
	DocURL string
}

// RelatedLocation is a secondary source location attached to a
// Diagnostic. It points the reader at the thing that explains the
// finding — for a schema violation, the line of the schema constraint.
// File may differ from the owning Diagnostic.File: the schema that a
// document violates lives in a separate proto.md, kind file, or config.
type RelatedLocation struct {
	// File is the path of the related source. It may be the linted
	// file itself or a separate schema source.
	File string

	// Line is the 1-based line of the related location.
	Line int

	// Column is the 1-based column, or 0 when only the line is known.
	Column int

	// Message describes why the location is related, e.g.
	// `schema requires one of: "open", "in-progress"`.
	Message string
}

// Explanation describes the provenance of a rule's effective config at
// the file that produced a diagnostic. It is attached to a Diagnostic
// when the CLI runs with --explain so the trailer / JSON output can
// name the rule and the source of each setting that contributed.
type Explanation struct {
	Rule   string
	Leaves []ExplanationLeaf
}

// ExplanationLeaf is one leaf setting and the layer that set its final
// value, formatted for surface output.
type ExplanationLeaf struct {
	Path   string
	Value  any
	Source string
}
