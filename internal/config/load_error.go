package config

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
)

// ConfigRuleID is the RuleID a config-load diagnostic carries. Config
// errors are not produced by a lint rule, so they use this fixed tag in
// the `<file>:<line>:<col> <rule> <message>` output slot.
const ConfigRuleID = "config"

// LoadError is a config load failure, positioned in the config file
// (or the `.mdsmith/` sidecar file) that holds the offending value
// when that position is known. Load and ParseBytes return it for every
// failure; Error() keeps the full wrapped message, while Message is the
// root issue alone, for display as a diagnostic.
type LoadError struct {
	// File is the file the position refers to: the config path passed
	// to Load, a sidecar file, or "" for ParseBytes input.
	File     string
	Message  string
	Severity lint.Severity
	Err      error

	// Line and Column are 1-based; Line is 0 when no position is known.
	// Column counts bytes, like every lint.Diagnostic column, so the
	// CLI prints and the LSP converts it as it does a Markdown
	// diagnostic's.
	Line   int
	Column int
}

// Error returns the full wrapped error text.
func (e *LoadError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying error chain.
func (e *LoadError) Unwrap() error { return e.Err }

// Positioned reports whether the error carries a line in File.
func (e *LoadError) Positioned() bool { return e.Line > 0 }

// Diagnostic renders the error as a lint diagnostic anchored on File.
func (e *LoadError) Diagnostic() lint.Diagnostic {
	return lint.Diagnostic{
		File:     e.File,
		RuleID:   ConfigRuleID,
		RuleName: ConfigRuleID,
		Severity: e.Severity,
		Message:  e.Message,
		Line:     e.Line,
		Column:   e.Column,
	}
}

// PositionedDiagnostic returns the diagnostic for a config load failure
// that carries a position in a named config file. ok is false for any
// other error, which callers report as plain text. The CLI prints the
// diagnostic and the language server publishes it on the config file.
func PositionedDiagnostic(err error) (d lint.Diagnostic, ok bool) {
	var le *LoadError
	if !errors.As(err, &le) || !le.Positioned() || le.File == "" {
		return lint.Diagnostic{}, false
	}
	return le.Diagnostic(), true
}

// positionError wraps a load failure in a LoadError, resolving the
// position of the Issue in err's chain. file is the config the bytes
// came from; resolver lazily builds the PositionResolver over those
// bytes (only an issue addressed by key path needs it). An issue that
// names a different file — a `.mdsmith/` sidecar — is resolved in that
// file instead.
func positionError(err error, file string, resolver func() PositionResolver) error {
	if err == nil {
		return nil
	}
	le := &LoadError{File: file, Message: err.Error(), Severity: lint.Error, Err: err}
	var iss *Issue
	if !errors.As(err, &iss) {
		return le
	}
	le.Message = iss.Message
	if iss.Severity != "" {
		le.Severity = iss.Severity
	}
	if iss.File != "" && iss.File != file {
		le.File = iss.File
		le.Line, le.Column = sidecarPosition(iss)
		return le
	}
	switch {
	case iss.Line > 0:
		le.Line, le.Column = iss.Line, iss.Column
	case len(iss.Path) > 0 && resolver != nil:
		if line, col, ok := resolver().Resolve(iss.Path); ok {
			le.Line, le.Column = line, col
		}
	}
	return le
}

// sidecarPosition positions an issue that lives in a `.mdsmith/`
// sidecar file. A sidecar holds the body of one entry of a top-level
// map — `.mdsmith/kinds/plan.yml` is the body of `kinds.plan` — so the
// first two key-path elements are dropped and the rest resolved inside
// the file. An issue addressed at the entry itself anchors on line 1.
func sidecarPosition(iss *Issue) (line, col int) {
	if iss.Line <= 0 && len(iss.Path) <= 2 {
		return 1, 1
	}
	data, err := readLimitedConfig(iss.File)
	if err != nil {
		if iss.Line > 0 {
			return iss.Line, iss.Column
		}
		return 0, 0
	}
	if iss.Line > 0 {
		return iss.Line, byteColumn(data, iss.Line, iss.Column)
	}
	line, col, ok := newYAMLResolver(data).Resolve(iss.Path[2:])
	if !ok {
		return 1, 1
	}
	return line, byteColumn(data, line, col)
}

// yamlPositionError is positionError for a config read from YAML text
// data. yaml.v3 counts a node's column in characters; the LoadError
// column is converted to the byte column in data, so text with
// multi-byte characters earlier on the line does not shift it. A
// position in a sidecar file is converted by sidecarPosition.
func yamlPositionError(err error, file string, data []byte) error {
	perr := positionError(err, file, yamlResolverFor(data))
	var le *LoadError
	if errors.As(perr, &le) && le.Positioned() && le.File == file {
		le.Column = byteColumn(data, le.Line, le.Column)
	}
	return perr
}

// byteColumn converts col, a 1-based column counted in characters on
// 1-based line of src, to the 1-based byte column of the same
// character. A column past the end of the line lands on the byte after
// it. A line or column outside src is returned unchanged.
func byteColumn(src []byte, line, col int) int {
	if line < 1 || col < 1 {
		return col
	}
	start := 0
	for n := 1; n < line; n++ {
		i := bytes.IndexByte(src[start:], '\n')
		if i < 0 {
			return col
		}
		start += i + 1
	}
	text := src[start:]
	if i := bytes.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	off := 0
	for c := 1; c < col && off < len(text); c++ {
		_, size := utf8.DecodeRune(text[off:])
		off += size
	}
	return off + 1
}

// yamlLineRe finds the `line N` marker yaml.v3 puts in parse and type
// errors ("yaml: line 3: ..." or "yaml: unmarshal errors:\n  line 3:").
var yamlLineRe = regexp.MustCompile(`\bline (\d+)\b`)

// yamlErrorIssue turns a yaml.v3 decode error into an Issue at the
// line the parser reported (column 1: yaml.v3 does not report one). An
// error that already is an Issue, or that names no line, passes through.
func yamlErrorIssue(err error) error {
	var iss *Issue
	if errors.As(err, &iss) {
		return err
	}
	m := yamlLineRe.FindStringSubmatch(err.Error())
	if m == nil {
		return err
	}
	n, _ := strconv.Atoi(m[1]) // \d+ always parses
	return &Issue{
		Message:  err.Error(),
		Severity: lint.Error,
		Err:      err,
		Line:     n,
		Column:   1,
	}
}
