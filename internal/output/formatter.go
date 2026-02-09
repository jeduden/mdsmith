package output

import (
	"io"

	"github.com/jeduden/tidymark/internal/lint"
)

// Formatter defines the interface for outputting diagnostics.
type Formatter interface {
	Format(w io.Writer, diagnostics []lint.Diagnostic) error
}
