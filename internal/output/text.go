package output

import (
	"fmt"
	"io"

	"github.com/jeduden/tidymark/internal/lint"
)

// TextFormatter outputs diagnostics in human-readable text format.
// When Color is true, the file location is printed in cyan and the rule ID in yellow.
type TextFormatter struct {
	Color bool
}

// Format writes each diagnostic as a single line in the pattern:
// file:line:col rule message
func (f *TextFormatter) Format(w io.Writer, diagnostics []lint.Diagnostic) error {
	for _, d := range diagnostics {
		var err error
		if f.Color {
			// file in cyan, rule in yellow
			_, err = fmt.Fprintf(w, "\033[36m%s:%d:%d\033[0m \033[33m%s\033[0m %s\n",
				d.File, d.Line, d.Column, d.RuleID, d.Message)
		} else {
			_, err = fmt.Fprintf(w, "%s:%d:%d %s %s\n",
				d.File, d.Line, d.Column, d.RuleID, d.Message)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
