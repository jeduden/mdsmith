package singleh1

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/yamlutil"
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

// collectH1s returns all authored H1 heading nodes in document order,
// excluding headings whose source line falls within a generated range.
// Line numbers are derived from headingLineStart so that empty headings
// with no text children (offset -1) are conservatively kept rather than
// misclassified via a stale fallback of line 1.
func collectH1s(f *lint.File) []*ast.Heading {
	var h1s []*ast.Heading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Level != 1 {
			return ast.WalkContinue, nil
		}
		if len(f.GeneratedRanges) > 0 {
			offset := headingLineStart(h, f.Source)
			if offset >= 0 {
				line := f.LineOfOffset(offset)
				for _, r := range f.GeneratedRanges {
					if r.Contains(line) {
						return ast.WalkContinue, nil
					}
				}
			}
			// offset == -1: position unknown; conservatively keep the heading
		}
		h1s = append(h1s, h)
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
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
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
	lineStart := headingLineStart(heading, source)
	if lineStart < 0 {
		return rep{}, false
	}
	if isATXHeadingAt(lineStart, source) {
		end := lineStart
		for end < len(source) && source[end] != '\n' {
			end++
		}
		line := source[lineStart:end]
		// Skip up to 3 CommonMark-allowed leading spaces, then the '#' run.
		i := 0
		for i < 3 && i < len(line) && line[i] == ' ' {
			i++
		}
		for i < len(line) && line[i] == '#' {
			i++
		}
		newText := string(line[:i]) + "#" + string(line[i:])
		return rep{start: lineStart, end: end, newText: newText}, true
	}

	// Setext heading: replace the underline line '===...' with '---...'
	textEnd := lineStart
	for textEnd < len(source) && source[textEnd] != '\n' {
		textEnd++
	}
	underlineStart := textEnd + 1
	underlineEnd := underlineStart
	for underlineEnd < len(source) && source[underlineEnd] != '\n' {
		underlineEnd++
	}
	underlineContent := source[underlineStart:underlineEnd]
	newUnderline := strings.ReplaceAll(string(underlineContent), "=", "-")
	newText := string(source[lineStart:underlineStart]) + newUnderline
	return rep{start: lineStart, end: underlineEnd, newText: newText}, true
}

// isATXHeadingAt checks whether source at lineStart begins with an ATX
// heading marker (0-3 leading spaces then '#').
func isATXHeadingAt(lineStart int, source []byte) bool {
	spaces := 0
	for spaces < 3 && lineStart+spaces < len(source) && source[lineStart+spaces] == ' ' {
		spaces++
	}
	pos := lineStart + spaces
	return pos < len(source) && source[pos] == '#'
}

// headingLineStart returns the byte offset of the start of the line
// containing the heading, or -1 if the position cannot be determined.
// goldmark normally sets Lines() for all heading types; when Lines() is
// empty (possible for ATX headings in some goldmark versions), falls back
// to the first descendant ast.Text segment (matching astutil.HeadingLine).
func headingLineStart(heading *ast.Heading, source []byte) int {
	var offset int
	if heading.Lines().Len() > 0 {
		offset = heading.Lines().At(0).Start
	} else {
		found := false
		_ = ast.Walk(heading, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering || n == heading {
				return ast.WalkContinue, nil
			}
			t, ok := n.(*ast.Text)
			if !ok {
				return ast.WalkContinue, nil
			}
			offset = t.Segment.Start
			found = true
			return ast.WalkStop, nil
		})
		if !found {
			return -1
		}
	}
	for offset > 0 && source[offset-1] != '\n' {
		offset--
	}
	return offset
}

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Demote extra H1 to H2" }
