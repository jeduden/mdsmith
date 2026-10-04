package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/output"
)

// printConfigError reports a config load failure on w. A failure that
// carries a position in a config file prints as a diagnostic —
// `<file>:<line>:<col> config <message>` — so editors and terminals can
// jump to the offending value; any other error keeps the plain
// `mdsmith: <error>` line. The caller still exits 2.
func printConfigError(w io.Writer, err error) {
	var le *config.LoadError
	if !errors.As(err, &le) || !le.Positioned() || le.File == "" {
		_, _ = fmt.Fprintf(w, "mdsmith: %v\n", err)
		return
	}
	d := le.Diagnostic()
	d.File = displayPath(d.File)
	f := &output.TextFormatter{}
	_ = f.Format(w, []lint.Diagnostic{d}) // best-effort write to stderr
}

// displayPath renders path relative to the working directory when it
// lies under it, matching how lint diagnostics print paths; otherwise
// path is returned unchanged.
func displayPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
