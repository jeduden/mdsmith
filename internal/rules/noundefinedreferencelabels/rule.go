// Package noundefinedreferencelabels implements MDS054, which flags
// reference-style links and images whose label has no matching link
// reference definition in the file.
package noundefinedreferencelabels

import (
	"bytes"
	"fmt"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
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

// WordlistTarget implements rule.WordlistConsumer: resolved `lists:`
// entries union into this rule's "placeholders" setting.
func (r *Rule) WordlistTarget() string { return "placeholders" }

var _ rule.WordlistConsumer = (*Rule)(nil)

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
	codeSpans := f.CodeSpanLiteralRanges()
	piLines := lint.CollectPIBlockLines(f)
	brs, brsBuf := collectBrackets(f.Source)
	defer releaseBrackets(brsBuf)

	var diags []lint.Diagnostic
	diags = append(diags, r.scanFullRefs(f, brs, defs, codeSpans, codeLines, piLines)...)
	diags = append(diags, r.scanCollapsedRefs(f, brs, defs, codeSpans, codeLines, piLines)...)
	if shortcut != shortcutCollapsedOnly {
		diags = append(diags, r.scanShortcutRefs(f, brs, defs, codeSpans, codeLines, piLines, shortcut)...)
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

// inCodeSpan reports whether offset falls inside any span. spans is
// f.CodeSpanLiteralRanges(), which the File memo emits in ascending
// document order, so a binary search replaces the linear scan the
// bracket-dense corpora paid per candidate.
func inCodeSpan(spans []lint.Range, offset int) bool {
	lo, hi := 0, len(spans)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case offset >= spans[mid].End:
			lo = mid + 1
		case offset < spans[mid].Start:
			hi = mid
		default:
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

// bracket is one maximal `[label]` occurrence: open indexes the
// `[`, [cs,ce) is the label, ca indexes just past the `]`. Labels
// never contain `[`, `]`, or a newline, so entries cannot overlap
// and collectBrackets' single pass enumerates exactly the entries a
// nextBracket scan from any position would find at or after it.
type bracket struct{ open, cs, ce, ca int }

// bracketBufPool recycles the per-Check bracket slice across files so
// the shared enumeration stays inside the rule's allocation budget —
// the same buffer-pool pattern as mdtext's extractTextBufPool.
var bracketBufPool = sync.Pool{New: func() any { return new([]bracket) }}

// collectBrackets enumerates every bracket entry in source once, so
// the three reference scanners share one pass instead of each
// re-walking the whole source through nextBracket. The returned slice
// is pool-backed: callers hand the second return back via
// releaseBrackets and must not touch the slice afterwards.
func collectBrackets(source []byte) ([]bracket, *[]bracket) {
	bufp := bracketBufPool.Get().(*[]bracket)
	out := (*bufp)[:0]
	pos := 0
	for {
		open, cs, ce, ca, ok := nextBracket(source, pos)
		if !ok {
			*bufp = out
			return out, bufp
		}
		out = append(out, bracket{open: open, cs: cs, ce: ce, ca: ca})
		pos = ca
	}
}

// maxPooledBrackets caps the capacity a returned bracket buffer may
// carry back into the pool. One pathologically bracket-dense file
// (the 2 MiB input cap allows ~1M two-byte "[]" pairs, ~32 MiB of
// entries) would otherwise pin that capacity in the pool for the
// process lifetime — the LSP runs this rule on every check. Oversized
// buffers are dropped for the GC instead.
const maxPooledBrackets = 1 << 16

// releaseBrackets returns a collectBrackets buffer to the pool,
// dropping buffers whose capacity exceeds maxPooledBrackets.
func releaseBrackets(bufp *[]bracket) {
	if cap(*bufp) > maxPooledBrackets {
		return
	}
	bracketBufPool.Put(bufp)
}

// shortcutLabelShaped reproduces the shortcutRE label class for one
// bracket entry: non-empty, first char not `^` (the bracket scanner
// already excludes `[`/`]`/newline inside labels), and not followed by
// `[` (a full/collapsed reference) or `(` (an inline link).
func shortcutLabelShaped(source []byte, b bracket) bool {
	if b.cs == b.ce || source[b.cs] == '^' {
		return false
	}
	if b.ca < len(source) {
		next := source[b.ca]
		if next == '[' || next == '(' {
			return false
		}
	}
	return true
}

// advanceBracket returns the first index at or after i whose entry
// opens at or after pos — the shared-list equivalent of calling
// nextBracket(source, pos).
func advanceBracket(brs []bracket, i, pos int) int {
	for i < len(brs) && brs[i].open < pos {
		i++
	}
	return i
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
	brs []bracket,
	defs []string,
	spans []lint.Range,
	codeLines, piLines map[int]struct{},
) []lint.Diagnostic {
	source := f.Source
	var diags []lint.Diagnostic
	i := 0
	for i < len(brs) {
		b := brs[i]
		open1, cs1, ce1, ca1 := b.open, b.cs, b.ce, b.ca
		// Must be immediately followed by another `[…]` — no
		// intervening characters per the regex `\]\[`.
		if ca1 >= len(source) || source[ca1] != '[' {
			i = advanceBracket(brs, i, ca1)
			continue
		}
		if i+1 >= len(brs) || brs[i+1].open != ca1 {
			i = advanceBracket(brs, i, ca1)
			continue
		}
		cs2, ce2, ca2 := brs[i+1].cs, brs[i+1].ce, brs[i+1].ca
		// Label must be non-empty (regex `[^\[\]\n]+`).
		if cs2 == ce2 {
			i = advanceBracket(brs, i, ca2)
			continue
		}
		line := f.LineOfOffset(open1)
		if lint.InCodeOrPI(codeLines, piLines, line) ||
			inCodeSpan(spans, open1) || isEscapedBracket(source, open1) {
			i = advanceBracket(brs, i, ca2)
			continue
		}
		// Skip footnote-like [^...][...].
		if ce1 > cs1 && source[cs1] == '^' {
			i = advanceBracket(brs, i, ca2)
			continue
		}
		label := source[cs2:ce2]
		if len(r.Placeholders) > 0 && placeholders.ContainsBodyToken(string(label), r.Placeholders) {
			i = advanceBracket(brs, i, ca2)
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
		i = advanceBracket(brs, i, ca2)
	}
	return diags
}

// scanCollapsedRefs walks source for `[label][]` patterns.
// Replaces collapsedRefRE. Same alloc-pruning motivation as
// scanFullRefs.
func (r *Rule) scanCollapsedRefs(
	f *lint.File,
	brs []bracket,
	defs []string,
	spans []lint.Range,
	codeLines, piLines map[int]struct{},
) []lint.Diagnostic {
	source := f.Source
	var diags []lint.Diagnostic
	i := 0
	for i < len(brs) {
		open, cs, ce, ca := brs[i].open, brs[i].cs, brs[i].ce, brs[i].ca
		// Label must be non-empty (regex `[^\[\]\n]+`).
		if cs == ce {
			i = advanceBracket(brs, i+1, ca)
			continue
		}
		// Must be immediately followed by `[]`.
		if ca+1 >= len(source) || source[ca] != '[' || source[ca+1] != ']' {
			i = advanceBracket(brs, i+1, ca)
			continue
		}
		line := f.LineOfOffset(open)
		if lint.InCodeOrPI(codeLines, piLines, line) ||
			inCodeSpan(spans, open) || isEscapedBracket(source, open) {
			i = advanceBracket(brs, i+1, ca+2)
			continue
		}
		text := source[cs:ce]
		if text[0] == '^' {
			i = advanceBracket(brs, i+1, ca+2)
			continue
		}
		if len(r.Placeholders) > 0 && placeholders.ContainsBodyToken(string(text), r.Placeholders) {
			i = advanceBracket(brs, i+1, ca+2)
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
		i = advanceBracket(brs, i+1, ca+2)
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
	brs []bracket,
	defs []string,
	spans []lint.Range,
	codeLines, piLines map[int]struct{},
	shortcutMode string,
) []lint.Diagnostic {
	source := f.Source
	// The definition-line set is only consulted for candidates that
	// survive every cheaper filter; on bracket-dense prose almost
	// none do, so the full-source scan is deferred until the first
	// survivor needs it.
	var defLines map[int]struct{}
	defLinesBuilt := false

	var diags []lint.Diagnostic
	i := 0
	for i < len(brs) {
		open, cs, ce, ca := brs[i].open, brs[i].cs, brs[i].ce, brs[i].ca
		if !shortcutLabelShaped(source, brs[i]) {
			i = advanceBracket(brs, i+1, ca)
			continue
		}
		label := source[cs:ce]
		isImage := open > 0 && source[open-1] == '!'
		// The heuristic is a pure byte scan, cheaper than the line and
		// code-span probes below; on bracket-dense prose it filters
		// almost every candidate, so it runs first. Pure reordering of
		// independent skip-filters — the surviving set is identical.
		if !isImage && shortcutMode == shortcutHeuristic && !looksLikeRefTarget(label) {
			i = advanceBracket(brs, i+1, ca)
			continue
		}
		line := f.LineOfOffset(open)
		if lint.InCodeOrPI(codeLines, piLines, line) ||
			inCodeSpan(spans, open) || isEscapedBracket(source, open) {
			i = advanceBracket(brs, i+1, ca)
			continue
		}
		if !defLinesBuilt {
			defLines = collectRefDefLines(source)
			defLinesBuilt = true
		}
		if _, ok := defLines[line]; ok {
			i = advanceBracket(brs, i+1, ca)
			continue
		}
		if len(r.Placeholders) > 0 && placeholders.ContainsBodyToken(string(label), r.Placeholders) {
			i = advanceBracket(brs, i+1, ca)
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
		i = advanceBracket(brs, i+1, ca)
	}
	return diags
}

// collectRefDefLines returns the set of 1-based line numbers that
// contain a reference definition `[label]: dest`. The byte-level
// scan replaces the per-line refDefStartRE.Match call (the regex
// dispatch was small but non-zero on the alloc-budget path).
func collectRefDefLines(source []byte) map[int]struct{} {
	lines := make(map[int]struct{})
	lineNum := 1
	start := 0
	// Walk line by line with bytes.IndexByte (SIMD) rather than a
	// non-vectorized byte-at-a-time scan; the final iteration (no newline)
	// processes the trailing segment, matching the old i==len(source) case.
	for start <= len(source) {
		nl := bytes.IndexByte(source[start:], '\n')
		end := len(source)
		if nl >= 0 {
			end = start + nl
		}
		if refDefLineStarts(source, start, end) {
			lines[lineNum] = struct{}{}
		}
		lineNum++
		if nl < 0 {
			break
		}
		start = end + 1
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
func looksLikeRefTarget(label []byte) bool {
	// ASCII fast path: one byte-wise pass covers the dominant case;
	// any high byte falls through to the rune-decoding walk so
	// Unicode letters and digits keep their exact semantics.
	ascii := true
	for i := 0; i < len(label); i++ {
		c := label[i]
		if c == ' ' || c == '\t' {
			return false
		}
		if c >= 0x80 {
			ascii = false
		}
	}
	if len(label) == 0 {
		return false
	}
	if ascii {
		c := label[0]
		startsWithLetter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !startsWithLetter {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if c >= '0' && c <= '9' || c == '-' || c == '_' {
				return true
			}
		}
		return false
	}
	first, _ := utf8.DecodeRune(label)
	if !unicode.IsLetter(first) {
		return false
	}
	for i := 0; i < len(label); {
		ch, size := utf8.DecodeRune(label[i:])
		if unicode.IsDigit(ch) || ch == '-' || ch == '_' {
			return true
		}
		i += size
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
