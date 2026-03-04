// Package gensection provides a reusable engine for marker-based
// generated-section rules. A Directive implementation supplies
// rule-specific validation and content generation, while the Engine
// handles marker parsing, YAML extraction, content comparison, and
// fix (replacement) logic.
package gensection

import "github.com/jeduden/mdsmith/internal/lint"

// ColumnConfig holds per-column width and wrapping configuration.
type ColumnConfig struct {
	MaxWidth int    // maximum width for the column content
	Wrap     string // "truncate" (default) or "br"
}

// Directive defines a generated-section rule that produces content
// from markers.
type Directive interface {
	// Name returns the directive/rule name used in markers
	// (e.g., "catalog"). Markers are derived as:
	//   start: "<?" + Name()
	//   end:   "<?/" + Name() + "?>"
	Name() string

	// RuleID returns the lint rule ID (e.g., "MDS019").
	RuleID() string

	// RuleName returns the lint rule name (e.g., "catalog").
	RuleName() string

	// Validate checks directive-specific parameters. Returns
	// diagnostics for invalid params.
	Validate(filePath string, line int, params map[string]string,
		columns map[string]ColumnConfig) []lint.Diagnostic

	// Generate produces the expected content between markers.
	// Returns content and any diagnostics.
	Generate(f *lint.File, filePath string, line int,
		params map[string]string,
		columns map[string]ColumnConfig) (string, []lint.Diagnostic)
}
