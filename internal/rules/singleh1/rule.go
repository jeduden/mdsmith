package singleh1

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{FrontMatterTitle: "title"})
}

// Rule checks that at most one H1 heading appears per file.
type Rule struct {
	FrontMatterTitle string // front-matter field that counts as an H1 (empty = disabled)
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS051" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "single-h1" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	h1s := collectH1s(f)

	hasFMTitle := r.FrontMatterTitle != "" && r.frontMatterHasTitle(f)

	var diags []lint.Diagnostic

	if hasFMTitle && len(h1s) > 0 {
		diags = append(diags, r.newDiag(f, astutil.HeadingLine(h1s[0], f),
			"h1 heading conflicts with front-matter title"))
		for _, h := range h1s[1:] {
			diags = append(diags, r.newDiag(f, astutil.HeadingLine(h, f),
				"extra H1 heading; only one H1 is allowed per file"))
		}
	} else if len(h1s) > 1 {
		for _, h := range h1s[1:] {
			diags = append(diags, r.newDiag(f, astutil.HeadingLine(h, f),
				"extra H1 heading; only one H1 is allowed per file"))
		}
	}

	return diags
}

// Fix implements rule.FixableRule. Demotes extra H1s to H2. Does not
// auto-fix front-matter title conflicts.
func (r *Rule) Fix(f *lint.File) []byte {
	h1s := collectH1s(f)

	hasFMTitle := r.FrontMatterTitle != "" && r.frontMatterHasTitle(f)

	// Determine which headings to demote.
	var toDemote []*ast.Heading
	if hasFMTitle {
		// The first H1 conflicts with front matter — no auto-fix for that.
		// Extra H1s beyond the first still get demoted.
		if len(h1s) > 1 {
			toDemote = h1s[1:]
		}
	} else if len(h1s) > 1 {
		toDemote = h1s[1:]
	}

	if len(toDemote) == 0 {
		return f.Source
	}

	result := make([]byte, len(f.Source))
	copy(result, f.Source)

	var reps []rep

	for _, h := range toDemote {
		if r, ok := buildDemoteReplacement(h, f.Source); ok {
			reps = append(reps, r)
		}
	}

	// Apply in reverse order to preserve byte offsets.
	for i := len(reps) - 1; i >= 0; i-- {
		rep := reps[i]
		before := result[:rep.start]
		after := result[rep.end:]
		result = append(before, append([]byte(rep.newText), after...)...)
	}

	return result
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "front-matter-title":
			str, ok := v.(string)
			if !ok {
				return fmt.Errorf("single-h1: front-matter-title must be a string, got %T", v)
			}
			r.FrontMatterTitle = str
		default:
			return fmt.Errorf("single-h1: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"front-matter-title": "title",
	}
}

var (
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)

// collectH1s returns all H1 heading nodes in document order.
func collectH1s(f *lint.File) []*ast.Heading {
	var h1s []*ast.Heading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if ok && h.Level == 1 {
			h1s = append(h1s, h)
		}
		return ast.WalkContinue, nil
	})
	return h1s
}

func (r *Rule) newDiag(f *lint.File, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

// frontMatterHasTitle reports whether the configured front-matter field is
// present and non-empty. It reads from f.FrontMatter when available, and
// falls back to extracting front matter directly from f.Source.
func (r *Rule) frontMatterHasTitle(f *lint.File) bool {
	fmBytes := f.FrontMatter
	if len(fmBytes) == 0 {
		// Integration tests call lint.NewFile which doesn't strip front matter;
		// extract it directly from source.
		fmBytes, _ = lint.StripFrontMatter(f.Source)
	}
	if len(fmBytes) == 0 {
		return false
	}
	yamlBytes := extractYAMLBody(fmBytes)
	if len(yamlBytes) == 0 {
		return false
	}
	if err := lint.RejectYAMLAliases(yamlBytes); err != nil {
		return false
	}
	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return false
	}
	v, ok := raw[r.FrontMatterTitle]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && s != ""
}

var fmDelim = []byte("---\n")

// extractYAMLBody trims the opening and closing --- delimiters from a
// StripFrontMatter-produced block and returns the raw YAML bytes.
func extractYAMLBody(fm []byte) []byte {
	body := bytes.TrimPrefix(fm, fmDelim)
	body = bytes.TrimSuffix(body, fmDelim)
	return body
}

type rep struct {
	start, end int
	newText    string
}

// buildDemoteReplacement returns the byte replacement needed to demote an
// H1 to H2. ATX: inserts a '#' after any leading spaces and the existing
// '#' run. Setext: replaces the '=' underline with '-'.
func buildDemoteReplacement(heading *ast.Heading, source []byte) (rep, bool) {
	if isATXHeading(heading, source) {
		start, end := atxHeadingLineRange(heading, source)
		line := source[start:end]
		// Skip up to 3 CommonMark-allowed leading spaces, then the '#' run.
		i := 0
		for i < len(line) && line[i] == ' ' {
			i++
		}
		for i < len(line) && line[i] == '#' {
			i++
		}
		newText := string(line[:i]) + "#" + string(line[i:])
		return rep{start: start, end: end, newText: newText}, true
	}

	// Setext heading: replace the underline line '===...' with '---...'
	textStart, underlineEnd := setextHeadingRange(heading, source)
	textEnd := textStart
	for textEnd < len(source) && source[textEnd] != '\n' {
		textEnd++
	}
	underlineStart := textEnd + 1
	underlineContent := source[underlineStart:underlineEnd]
	newUnderline := strings.ReplaceAll(string(underlineContent), "=", "-")
	newText := string(source[textStart:underlineStart]) + newUnderline
	return rep{start: textStart, end: underlineEnd, newText: newText}, true
}

// isATXHeading returns true if the heading uses ATX style (# prefix).
// CommonMark allows 0-3 leading spaces before the # marker.
func isATXHeading(heading *ast.Heading, source []byte) bool {
	start := headingLineStart(heading, source)
	spaces := 0
	for spaces < 3 && start+spaces < len(source) && source[start+spaces] == ' ' {
		spaces++
	}
	start += spaces
	return start < len(source) && source[start] == '#'
}

// headingLineStart returns the byte offset of the start of the line
// containing the heading. goldmark sets Lines() for all heading types,
// so Lines().At(0).Start provides the content offset; we rewind to the
// line-start to include any leading-space or # prefix bytes.
func headingLineStart(heading *ast.Heading, source []byte) int {
	offset := heading.Lines().At(0).Start
	for offset > 0 && source[offset-1] != '\n' {
		offset--
	}
	return offset
}

// atxHeadingLineRange returns the [start, end) byte range of an ATX heading
// line (not including the trailing newline).
func atxHeadingLineRange(heading *ast.Heading, source []byte) (int, int) {
	start := headingLineStart(heading, source)
	end := start
	for end < len(source) && source[end] != '\n' {
		end++
	}
	return start, end
}

// setextHeadingRange returns the byte range of a setext heading: from the
// start of the text line to the end of the underline line (not including
// the trailing newline of the underline).
func setextHeadingRange(heading *ast.Heading, source []byte) (int, int) {
	textStart := headingLineStart(heading, source)
	textEnd := textStart
	for textEnd < len(source) && source[textEnd] != '\n' {
		textEnd++
	}
	underlineStart := textEnd + 1
	underlineEnd := underlineStart
	for underlineEnd < len(source) && source[underlineEnd] != '\n' {
		underlineEnd++
	}
	return textStart, underlineEnd
}
