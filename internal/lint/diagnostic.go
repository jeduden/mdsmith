package lint

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Diagnostic struct {
	File     string
	Line     int
	Column   int
	RuleID   string
	RuleName string
	Severity Severity
	Message  string
}
