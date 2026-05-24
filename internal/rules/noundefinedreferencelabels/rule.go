// Package noundefinedreferencelabels implements MDS054, which flags
// reference-style links and images whose label has no matching link
// reference definition in the file.
package noundefinedreferencelabels

import (
	"bytes"
	"fmt"
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
	// Every shape this rule flags starts with `[`; a file without
	// brackets cannot trigger MDS054, so the early-exit skips the
	// per-Check helper allocations (defs lookup, code-line maps,
	// code-span walk) entirely. Most prose files take this branch.
	if !bytes.ContainsRune(f.Source, '[') {
		return nil
	}

	shortcut := r.Shortcut
	if shortcut == "" {
		shortcut = shortcutHeuristic
	}

	defs := collectNormalisedDefs(f)
	codeLines := lint.CollectCodeBlockLines(f)
	codeSpans := collectCodeSpanRanges(f)
	piLines := lint.CollectPIBlockLines(f)

	var diags []lint.Diagnostic
	diags = append(diags, r.scanFullRefs(f, defs, codeSpans, codeLines, piLines)...)
	diags = append(diags, r.scanCollapsedRefs(f, defs, codeSpans, codeLines, piLines)...)
	if shortcut != shortcutCollapsedOnly {
		diags = append(diags, r.scanShortcutRefs(f, defs, codeSpans, codeLines, piLines, shortcut)...)
	}

	return diags
}

// collectNormalisedDefs returns the CommonMark-normalised labels of
// every link reference definition in f. A single string slice
// (sized exactly to len(refs)) replaces the previous map-of-
// normalised-labels so the per-Check defs build pays one slice
// header + N strings rather than one map header + bucket + N
// strings + a grow alloc on insert. labelDefined linear-scans the
// slice — small-N is the universal case for a single file, and
// the cache-friendly stride beats the map on the fixture's 1-ref
// load (plan 195 task 7).
func collectNormalisedDefs(f *lint.File) []string {
	refs := f.LinkReferences()
	if len(refs) == 0 {
		return nil
	}
	defs := make([]string, len(refs))
	for i, ref := range refs {
		defs[i] = normalizeLabel(ref.Label())
	}
	return defs
}

// labelDefined reports whether normalised matches any label in defs.
// defs is already CommonMark-normalised by collectNormalisedDefs.
func labelDefined(defs []string, normalised string) bool {
	for _, d := range defs {
		if d == normalised {
			return true
		}
	}
	return false
}

// normalizeLabel applies CommonMark reference label normalization.
// goldmark's util.ToLinkReference folds case and collapses whitespace
// and already returns a string, so this is a thin alias kept for
// call-site readability.
func normalizeLabel(raw []byte) string {
	return util.ToLinkReference(raw)
}

// byteRange is a half-open [start, end) byte range.
type byteRange struct{ start, end int }

// collectCodeSpanRanges returns byte ranges of inline code spans.
// Code spans are inline nodes; their content is accessed via child
// Text nodes. The walk uses a recursive helper rather than
// ast.Walk so the per-Check closure box ast.Walk would otherwise
// allocate is shed — plan 195 task 7.
func collectCodeSpanRanges(f *lint.File) []byteRange {
	var out []byteRange
	collectCodeSpanRangesInto(f.AST, f.Source, &out)
	return out
}

// collectCodeSpanRangesInto descends node n and appends the byte
// range of every *ast.CodeSpan to out, extending each range
// outward to include the surrounding backticks (the regex-based
// scanners later in this file want the full literal span).
// Recursive descent keeps the helper closure-free.
func collectCodeSpanRangesInto(n ast.Node, source []byte, out *[]byteRange) {
	if n == nil {
		return
	}
	if _, ok := n.(*ast.CodeSpan); ok {
		first, last := codeSpanTextBounds(n)
		if first >= 0 {
			start := first
			for start > 0 && source[start-1] == '`' {
				start--
			}
			end := last
			for end < len(source) && source[end] == '`' {
				end++
			}
			*out = append(*out, byteRange{start, end})
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectCodeSpanRangesInto(c, source, out)
	}
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

// nextBracket finds the next `[X]` occurrence starting at or after pos.
// Returns:
//   - open: index of the `[`
//   - contentStart: index right after `[`
//   - contentEnd: index of the `]`
//   - closeAfter: index right after `]`
//   - ok: true on success, false at EOF
//
// The label content cannot contain a nested `[`, `]`, or newline —
// matches the regex character class `[^\[\]\n]*` the previous form
// used. Used to replace fullRefRE, collapsedRefRE, and shortcutRE
// with byte-level scanning so the rule no longer pays the per-call
// regex result-slice + per-match `[]int` allocations — plan 195
// task 7. Empty contents (`[]`) return contentStart == contentEnd;
// the caller decides whether that is valid for the pattern.
func nextBracket(source []byte, pos int) (open, contentStart, contentEnd, closeAfter int, ok bool) {
	for pos < len(source) {
		if source[pos] != '[' {
			pos++
			continue
		}
		open = pos
		contentStart = pos + 1
		i := contentStart
		for i < len(source) && source[i] != ']' && source[i] != '[' && source[i] != '\n' {
			i++
		}
		if i < len(source) && source[i] == ']' {
			return open, contentStart, i, i + 1, true
		}
		// `[` opened but did not close on this line (or nested `[`);
		// advance past the orphan `[` and keep scanning.
		pos++
	}
	return 0, 0, 0, 0, false
}

// scanFullRefs walks source for `[text][label]` patterns. The byte
// scanner replaces fullRefRE, which on the gate fixture allocated
// ~2 objects (the regex result slice and a per-match `[]int`).
// Mirrors the original semantics: skip footnote-style `[^…][…]`,
// skip empty labels (the regex's `[^\[\]\n]+` required ≥ 1 char in
// the label, but allowed an empty text), and skip lines that are
// excluded or code-spanned or escape-prefixed.
func (r *Rule) scanFullRefs(
	f *lint.File,
	defs []string,
	spans []byteRange,
	codeLines, piLines map[int]bool,
) []lint.Diagnostic {
	source := f.Source
	var diags []lint.Diagnostic
	pos := 0
	for {
		open1, cs1, ce1, ca1, ok := nextBracket(source, pos)
		if !ok {
			break
		}
		// Must be immediately followed by another `[…]` — no
		// intervening characters per the regex `\]\[`.
		if ca1 >= len(source) || source[ca1] != '[' {
			pos = ca1
			continue
		}
		open2, cs2, ce2, ca2, ok2 := nextBracket(source, ca1)
		if !ok2 || open2 != ca1 {
			pos = ca1
			continue
		}
		// Label must be non-empty (regex `[^\[\]\n]+`).
		if cs2 == ce2 {
			pos = ca2
			continue
		}
		line := f.LineOfOffset(open1)
		if codeLines[line] || piLines[line] ||
			inCodeSpan(spans, open1) || isEscapedBracket(source, open1) {
			pos = ca2
			continue
		}
		// Skip footnote-like [^...][...].
		if ce1 > cs1 && source[cs1] == '^' {
			pos = ca2
			continue
		}
		label := source[cs2:ce2]
		if len(r.Placeholders) > 0 && placeholders.ContainsBodyToken(string(label), r.Placeholders) {
			pos = ca2
			continue
		}
		normalized := normalizeLabel(label)
		if !labelDefined(defs, normalized) {
			col := f.ColumnOfOffset(open1)
			if open1 > 0 && source[open1-1] == '!' {
				col = f.ColumnOfOffset(open1 - 1)
			}
			diags = append(diags, r.diag(f.Path, line, col,
				fmt.Sprintf("reference label %q has no matching link reference definition", string(label))))
		}
		pos = ca2
	}
	return diags
}

// scanCollapsedRefs walks source for `[label][]` patterns.
// Replaces collapsedRefRE. Same alloc-pruning motivation as
// scanFullRefs.
func (r *Rule) scanCollapsedRefs(
	f *lint.File,
	defs []string,
	spans []byteRange,
	codeLines, piLines map[int]bool,
) []lint.Diagnostic {
	source := f.Source
	var diags []lint.Diagnostic
	pos := 0
	for {
		open, cs, ce, ca, ok := nextBracket(source, pos)
		if !ok {
			break
		}
		// Label must be non-empty (regex `[^\[\]\n]+`).
		if cs == ce {
			pos = ca
			continue
		}
		// Must be immediately followed by `[]`.
		if ca+1 >= len(source) || source[ca] != '[' || source[ca+1] != ']' {
			pos = ca
			continue
		}
		line := f.LineOfOffset(open)
		if codeLines[line] || piLines[line] ||
			inCodeSpan(spans, open) || isEscapedBracket(source, open) {
			pos = ca + 2
			continue
		}
		text := source[cs:ce]
		if text[0] == '^' {
			pos = ca + 2
			continue
		}
		if len(r.Placeholders) > 0 && placeholders.ContainsBodyToken(string(text), r.Placeholders) {
			pos = ca + 2
			continue
		}
		normalized := normalizeLabel(text)
		if !labelDefined(defs, normalized) {
			col := f.ColumnOfOffset(open)
			if open > 0 && source[open-1] == '!' {
				col = f.ColumnOfOffset(open - 1)
			}
			diags = append(diags, r.diag(f.Path, line, col,
				fmt.Sprintf("reference label %q has no matching link reference definition", string(text))))
		}
		pos = ca + 2
	}
	return diags
}

// scanShortcutRefs walks source for `[label]` patterns whose context
// is neither a full nor collapsed reference. Replaces shortcutRE.
// The caller's filter set is unchanged: not on an excluded line,
// not in a code span, not escaped, not on a reference-definition
// line, and (under heuristic shortcutMode) only when the label
// looks plausibly like a reference target.
func (r *Rule) scanShortcutRefs(
	f *lint.File,
	defs []string,
	spans []byteRange,
	codeLines, piLines map[int]bool,
	shortcutMode string,
) []lint.Diagnostic {
	source := f.Source
	// Build the set of definition-line numbers via a byte scan; the
	// previous regex-driven helper allocated the regex result plus a
	// per-line `string` for the destination-presence check on every
	// candidate line.
	defLines := collectRefDefLines(source)

	var diags []lint.Diagnostic
	pos := 0
	for {
		open, cs, ce, ca, ok := nextBracket(source, pos)
		if !ok {
			break
		}
		// Reproduce the shortcutRE label class: non-empty, first char
		// not `^`/`[`/`]`/`\n` (the bracket scanner already excludes
		// `[`/`]`/`\n` inside the label, so only the `^` check is
		// new here).
		if cs == ce || source[cs] == '^' {
			pos = ca
			continue
		}
		// Skip if followed by `[` (full/collapsed ref) or `(` (inline link).
		if ca < len(source) {
			next := source[ca]
			if next == '[' || next == '(' {
				pos = ca
				continue
			}
		}
		line := f.LineOfOffset(open)
		if codeLines[line] || piLines[line] ||
			inCodeSpan(spans, open) || isEscapedBracket(source, open) {
			pos = ca
			continue
		}
		if defLines[line] {
			pos = ca
			continue
		}
		label := source[cs:ce]
		if len(r.Placeholders) > 0 && placeholders.ContainsBodyToken(string(label), r.Placeholders) {
			pos = ca
			continue
		}
		isImage := open > 0 && source[open-1] == '!'
		if !isImage && shortcutMode == shortcutHeuristic && !looksLikeRefTarget(string(label)) {
			pos = ca
			continue
		}
		normalized := normalizeLabel(label)
		if !labelDefined(defs, normalized) {
			col := f.ColumnOfOffset(open)
			if isImage {
				col = f.ColumnOfOffset(open - 1)
			}
			diags = append(diags, r.diag(f.Path, line, col,
				fmt.Sprintf("reference label %q has no matching link reference definition", string(label))))
		}
		pos = ca
	}
	return diags
}

// collectRefDefLines returns the set of 1-based line numbers that
// contain a reference definition `[label]: dest`. The byte-level
// scan replaces the per-line refDefStartRE.Match call (the regex
// dispatch was small but non-zero on the alloc-budget path).
func collectRefDefLines(source []byte) map[int]bool {
	lines := make(map[int]bool)
	lineNum := 1
	start := 0
	for i := 0; i <= len(source); i++ {
		if i == len(source) || source[i] == '\n' {
			if refDefLineStarts(source, start, i) {
				lines[lineNum] = true
			}
			lineNum++
			start = i + 1
		}
	}
	return lines
}

// refDefLineStarts reports whether source[lineStart:lineEnd] looks
// like a CommonMark reference definition: 0-3 leading spaces, then
// `[`, then any non-`]` content, then `]:`. The destination is not
// validated here because the caller only needs to know whether the
// line LOOKS like a refdef for the purpose of suppressing the
// shortcut-ref scan; an over-greedy match would only suppress a
// non-existent diag.
func refDefLineStarts(source []byte, lineStart, lineEnd int) bool {
	j := lineStart
	spaces := 0
	for j < lineEnd && source[j] == ' ' && spaces < 3 {
		j++
		spaces++
	}
	if j >= lineEnd || source[j] != '[' {
		return false
	}
	for j < lineEnd && source[j] != ']' {
		j++
	}
	if j >= lineEnd {
		return false
	}
	// Already on `]`; require `:` after, possibly with whitespace.
	j++
	for j < lineEnd && (source[j] == ' ' || source[j] == '\t') {
		j++
	}
	return j < lineEnd && source[j] == ':'
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
