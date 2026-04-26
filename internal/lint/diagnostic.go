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
}
