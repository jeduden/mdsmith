// Package noundefinedreferencelabels implements MDS054, which flags
// reference-style links and images whose label has no matching link
// reference definition in the file.
package noundefinedreferencelabels

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags reference-style links and images with undefined labels.
type Rule struct {
	Shortcut     string   // "heuristic" | "always" | "collapsed-only"
	Placeholders []string // placeholder tokens treated as opaque
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS054" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-undefined-reference-labels" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return true }

const (
	shortcutHeuristic     = "heuristic"
	shortcutAlways        = "always"
	shortcutCollapsedOnly = "collapsed-only"
)

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	shortcut := r.Shortcut
	if shortcut == "" {
		shortcut = shortcutHeuristic
	}

	defs := collectReferenceDefs(f)
	codeLines := lint.CollectCodeBlockLines(f)
	codeSpans := collectCodeSpanRanges(f)
	piLines := lint.CollectPIBlockLines(f)

	excluded := func(line int) bool {
		return codeLines[line] || piLines[line]
	}

	var diags []lint.Diagnostic
	diags = append(diags, r.scanFullRefs(f, defs, codeSpans, excluded)...)
	diags = append(diags, r.scanCollapsedRefs(f, defs, codeSpans, excluded)...)
	if shortcut != shortcutCollapsedOnly {
		diags = append(diags, r.scanShortcutRefs(f, defs, codeSpans, excluded, shortcut)...)
	}

	return diags
}

// collectReferenceDefs returns the set of normalized labels for which a
// link reference definition exists, reading the definitions goldmark
// already collected during the file's single parse (see
// lint.File.LinkReferences) rather than re-parsing the source.
func collectReferenceDefs(f *lint.File) map[string]bool {
	refs := f.LinkReferences()
	defs := make(map[string]bool, len(refs))
	for _, ref := range refs {
		defs[normalizeLabel(ref.Label())] = true
	}
	return defs
}

// normalizeLabel applies CommonMark reference label normalization.
// goldmark's util.ToLinkReference folds case and collapses whitespace.
func normalizeLabel(raw []byte) string {
	return string(util.ToLinkReference(raw))
}

// byteRange is a half-open [start, end) byte range.
type byteRange struct{ start, end int }

// collectCodeSpanRanges returns byte ranges of inline code spans.
// Code spans are inline nodes; their content is accessed via child Text nodes.
func collectCodeSpanRanges(f *lint.File) []byteRange {
	var out []byteRange
	source := f.Source
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		_, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		// Collect the start of the first and end of the last Text child.
		first, last := codeSpanTextBounds(n)
		if first < 0 {
			return ast.WalkContinue, nil
		}
		// Extend outward to include the surrounding backticks.
		start := first
		for start > 0 && source[start-1] == '`' {
			start--
		}
		end := last
		for end < len(source) && source[end] == '`' {
			end++
		}
		out = append(out, byteRange{start, end})
		return ast.WalkContinue, nil
	})
	return out
}

func codeSpanTextBounds(n ast.Node) (first, last int) {
	first = -1
	last = -1
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		if first < 0 {
			first = t.Segment.Start
		}
		last = t.Segment.Stop
	}
	return first, last
}

func inCodeSpan(spans []byteRange, offset int) bool {
	for _, r := range spans {
		if offset >= r.start && offset < r.end {
			return true
		}
	}
	return false
}

// isEscapedBracket reports whether the '[' at source[pos] is preceded by an
// odd number of backslashes, making it a CommonMark backslash escape rather
// than the start of a link or image.
func isEscapedBracket(source []byte, pos int) bool {
	n := 0
	for pos-1-n >= 0 && source[pos-1-n] == '\\' {
		n++
	}
	return n%2 == 1
}

// fullRefRE matches [text][label] — full reference link/image.
// Group 1: text, Group 2: label (non-empty).
// Does not match [^...] (footnotes) via the label check below.
var fullRefRE = regexp.MustCompile(`\[([^\[\]\n]*)\]\[([^\[\]\n]+)\]`)

// collapsedRefRE matches [label][] — collapsed reference.
// Group 1: label (the text, which becomes the label).
var collapsedRefRE = regexp.MustCompile(`\[([^\[\]\n]+)\]\[\]`)

func (r *Rule) scanFullRefs(
	f *lint.File,
	defs map[string]bool,
	spans []byteRange,
	excluded func(int) bool,
) []lint.Diagnostic {
	source := f.Source
	var diags []lint.Diagnostic
	for _, m := range fullRefRE.FindAllSubmatchIndex(source, -1) {
		start := m[0]
		line := f.LineOfOffset(start)
		if excluded(line) || inCodeSpan(spans, start) || isEscapedBracket(source, start) {
			continue
		}
		// The label comes from group 2 (the second bracket pair).
		label := source[m[4]:m[5]]
		// Skip footnote-like [^...][...]: text starting with '^' is a footnote reference.
		if len(source[m[2]:m[3]]) > 0 && source[m[2]] == '^' {
			continue
		}
		normalized := normalizeLabel(label)
		if placeholders.ContainsBodyToken(string(label), r.Placeholders) {
			continue
		}
		if !defs[normalized] {
			col := f.ColumnOfOffset(start)
			// Adjust start for image prefix '!'
			if start > 0 && source[start-1] == '!' {
				col = f.ColumnOfOffset(start - 1)
			}
			diags = append(diags, r.diag(f.Path, line, col,
				fmt.Sprintf("reference label %q has no matching link reference definition", string(label))))
		}
	}
	return diags
}

func (r *Rule) scanCollapsedRefs(
	f *lint.File,
	defs map[string]bool,
	spans []byteRange,
	excluded func(int) bool,
) []lint.Diagnostic {
	source := f.Source
	var diags []lint.Diagnostic
	for _, m := range collapsedRefRE.FindAllSubmatchIndex(source, -1) {
		start := m[0]
		line := f.LineOfOffset(start)
		if excluded(line) || inCodeSpan(spans, start) || isEscapedBracket(source, start) {
			continue
		}
		text := source[m[2]:m[3]]
		// Skip footnotes
		if len(text) > 0 && text[0] == '^' {
			continue
		}
		normalized := normalizeLabel(text)
		if placeholders.ContainsBodyToken(string(text), r.Placeholders) {
			continue
		}
		if !defs[normalized] {
			col := f.ColumnOfOffset(start)
			if start > 0 && source[start-1] == '!' {
				col = f.ColumnOfOffset(start - 1)
			}
			diags = append(diags, r.diag(f.Path, line, col,
				fmt.Sprintf("reference label %q has no matching link reference definition", string(text))))
		}
	}
	return diags
}

// shortcutRE matches [label] forms. The caller filters out cases that are
// followed by '[' or '(' (full/collapsed refs and inline links), lines that
// are reference definitions, and — for image shortcuts — checks for '!' prefix.
var shortcutRE = regexp.MustCompile(`\[([^\[\]\n^][^\[\]\n]*)\]`)

// refDefStartRE detects a reference definition at a given line start.
var refDefStartRE = regexp.MustCompile(`^[ ]{0,3}\[`)

func (r *Rule) scanShortcutRefs(
	f *lint.File,
	defs map[string]bool,
	spans []byteRange,
	excluded func(int) bool,
	shortcutMode string,
) []lint.Diagnostic {
	source := f.Source
	// Build set of definition line start offsets to skip.
	defLines := collectRefDefLines(source)

	var diags []lint.Diagnostic
	for _, m := range shortcutRE.FindAllSubmatchIndex(source, -1) {
		start := m[0]
		end := m[1]
		line := f.LineOfOffset(start)

		if excluded(line) || inCodeSpan(spans, start) || isEscapedBracket(source, start) {
			continue
		}
		// Skip if followed by '[' (full/collapsed ref) or '(' (inline link).
		// Reference definition lines are excluded separately via defLines.
		if end < len(source) {
			next := source[end]
			if next == '[' || next == '(' {
				continue
			}
		}
		// Skip if this is a reference definition line.
		if defLines[line] {
			continue
		}
		label := source[m[2]:m[3]]
		normalized := normalizeLabel(label)
		if placeholders.ContainsBodyToken(string(label), r.Placeholders) {
			continue
		}
		// Detect image shortcut ![label]: the '!' immediately precedes '['.
		isImage := start > 0 && source[start-1] == '!'
		// Under heuristic mode, only flag if the label looks like a reference
		// target — but always flag image shortcuts since '!' makes intent clear.
		if !isImage && shortcutMode == shortcutHeuristic && !looksLikeRefTarget(string(label)) {
			continue
		}
		if !defs[normalized] {
			col := f.ColumnOfOffset(start)
			if isImage {
				col = f.ColumnOfOffset(start - 1)
			}
			diags = append(diags, r.diag(f.Path, line, col,
				fmt.Sprintf("reference label %q has no matching link reference definition", string(label))))
		}
	}
	return diags
}

// collectRefDefLines returns the set of 1-based line numbers that contain
// a reference definition `[label]: dest`.
func collectRefDefLines(source []byte) map[int]bool {
	lines := make(map[int]bool)
	lineNum := 1
	start := 0
	for i := 0; i <= len(source); i++ {
		if i == len(source) || source[i] == '\n' {
			line := source[start:i]
			if refDefStartRE.Match(line) {
				// Check if it has `: ` after the closing `]`
				if idx := indexByte(line, ']'); idx >= 0 {
					rest := strings.TrimLeft(string(line[idx+1:]), " \t")
					if strings.HasPrefix(rest, ":") {
						lines[lineNum] = true
					}
				}
			}
			lineNum++
			start = i + 1
		}
	}
	return lines
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// looksLikeRefTarget reports whether label looks like a reference target
// under the heuristic: starts with a letter, no spaces, and contains at
// least one digit, hyphen, or underscore. Requiring a leading letter avoids
// false positives on regex character classes like [0-9] or [a-z].
func looksLikeRefTarget(label string) bool {
	if strings.ContainsAny(label, " \t") {
		return false
	}
	runes := []rune(label)
	if len(runes) == 0 || !unicode.IsLetter(runes[0]) {
		return false
	}
	for _, ch := range runes {
		if unicode.IsDigit(ch) || ch == '-' || ch == '_' {
			return true
		}
	}
	return false
}

func (r *Rule) diag(path string, line, col int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "shortcut":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("no-undefined-reference-labels: shortcut must be a string, got %T", v)
			}
			switch s {
			case shortcutHeuristic, shortcutAlways, shortcutCollapsedOnly:
			default:
				return fmt.Errorf(
					"no-undefined-reference-labels: shortcut must be %q, %q, or %q, got %q",
					shortcutHeuristic, shortcutAlways, shortcutCollapsedOnly, s,
				)
			}
			r.Shortcut = s
		case "placeholders":
			toks, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"no-undefined-reference-labels: placeholders must be a list of strings, got %T", v,
				)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("no-undefined-reference-labels: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf("no-undefined-reference-labels: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"shortcut":     shortcutHeuristic,
		"placeholders": []string{},
	}
}

// SettingMergeMode implements rule.ListMerger.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "placeholders" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

func toStringSlice(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		out := make([]string, len(list))
		copy(out, list)
		return out, true
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
