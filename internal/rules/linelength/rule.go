package linelength

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{
		Max:     80,
		Exclude: []string{"code-blocks", "tables", "urls"},
	})
}

// Rule checks that no line exceeds the configured maximum length.
// Lines matching categories in Exclude are skipped. Valid exclude
// values: "code-blocks", "tables", "urls".
type Rule struct {
	Max          int
	HeadingMax   *int
	CodeBlockMax *int
	Stern        bool
	Exclude      []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS001" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "line-length" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "line" }

// LineCapable implements rule.LineCapable: line-length reads f.Lines, the
// classifier-backed code-block set (CollectCodeBlockLines), and the table
// byte-scan — all byte-identical to the AST on the flat Layer-0 path. It
// reports false when a per-heading limit is configured, because the
// classifier's heading-line set is NOT guaranteed byte-identical to the AST
// walk (the AST path's collectHeadingLines has container-prefix and
// multi-line-setext quirks the flat pass does not replicate), so a
// heading-max config stays on the AST path. The engine consults the
// configured instance, so HeadingMax is set by the time this is called
// (plan 2606142147).
func (r *Rule) LineCapable() bool { return r.HeadingMax == nil }

// isExcluded returns true if the given category is in the Exclude list.
func (r *Rule) isExcluded(category string) bool {
	for _, e := range r.Exclude {
		if e == category {
			return true
		}
	}
	return false
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		if err := r.applySetting(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rule) applySetting(k string, v any) error {
	switch k {
	case "max":
		return r.applyMax(v)
	case "heading-max":
		return r.applyPositiveIntPtr(v, "heading-max", &r.HeadingMax)
	case "code-block-max":
		return r.applyPositiveIntPtr(v, "code-block-max", &r.CodeBlockMax)
	case "stern":
		return r.applyStern(v)
	case "exclude":
		return r.applyExclude(v)
	case "strict":
		return r.applyStrict(v)
	default:
		return fmt.Errorf("line-length: unknown setting %q", k)
	}
}

func (r *Rule) applyMax(v any) error {
	n, ok := settings.ToInt(v)
	if !ok {
		return fmt.Errorf("line-length: max must be an integer, got %T", v)
	}
	r.Max = n
	return nil
}

func (r *Rule) applyPositiveIntPtr(v any, name string, target **int) error {
	n, ok := settings.ToInt(v)
	if !ok {
		return fmt.Errorf("line-length: %s must be an integer, got %T", name, v)
	}
	if n <= 0 {
		return fmt.Errorf("line-length: %s must be positive, got %d", name, n)
	}
	*target = &n
	return nil
}

func (r *Rule) applyStern(v any) error {
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("line-length: stern must be a bool, got %T", v)
	}
	r.Stern = b
	return nil
}

func (r *Rule) applyExclude(v any) error {
	list, ok := settings.ToStringSlice(v)
	if !ok {
		return fmt.Errorf("line-length: exclude must be a list of strings, got %T", v)
	}
	for _, item := range list {
		if !isValidExclude(item) {
			return fmt.Errorf("line-length: invalid exclude value %q (valid: code-blocks, tables, urls)", item)
		}
	}
	r.Exclude = list
	return nil
}

func (r *Rule) applyStrict(v any) error {
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("line-length: strict must be a bool, got %T", v)
	}
	// Deprecation shim: translate strict to exclude.
	if b {
		r.Exclude = []string{}
	} else {
		r.Exclude = []string{"code-blocks", "tables", "urls"}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max":     80,
		"exclude": []string{"code-blocks", "tables", "urls"},
		"stern":   false,
	}
}

// lineCategories holds the line-set lookups Check needs to decide
// per-line max and per-line skip. Each map is nil when the active
// settings do not need it — reads from a nil map return false, which
// is exactly the "line is not in this category" answer the rest of
// the rule wants, so the absent-by-default state costs no allocation
// and no per-line branch. Pre-creating empty maps to "look uniform"
// added three wasted allocations per Check before plan 195.
type lineCategories struct {
	code    map[int]struct{}
	table   map[int]struct{}
	heading map[int]struct{}
}

func (r *Rule) buildCategories(f *lint.File) lineCategories {
	var lc lineCategories
	if r.isExcluded("code-blocks") || r.CodeBlockMax != nil {
		lc.code = lint.CollectCodeBlockLines(f)
	}
	if r.isExcluded("tables") {
		lc.table = collectTableLines(f)
	}
	if r.HeadingMax != nil {
		lc.heading = collectHeadingLines(f)
	}
	return lc
}

// activeMax returns the effective maximum for a line given its categories.
func (r *Rule) activeMax(baseMax int, lc lineCategories, lineNum int) int {
	if _, ok := lc.heading[lineNum]; ok && r.HeadingMax != nil {
		return *r.HeadingMax
	}
	if _, ok := lc.code[lineNum]; ok && r.CodeBlockMax != nil {
		return *r.CodeBlockMax
	}
	return baseMax
}

// isSkipped returns true if the line should be excluded from checking.
func (r *Rule) isSkipped(line []byte, lineNum, limit int, lc lineCategories) bool {
	if r.isExcluded("code-blocks") {
		if _, ok := lc.code[lineNum]; ok {
			return true
		}
	}
	if r.isExcluded("tables") {
		if _, ok := lc.table[lineNum]; ok {
			return true
		}
	}
	if r.isExcluded("urls") && isURLOnlyLine(line) {
		return true
	}
	if r.Stern && !hasSpacePastLimit(line, limit) {
		return true
	}
	return false
}

// isURLOnlyLine reports whether line, after trimming whitespace,
// is a single http(s) URL with no internal ASCII whitespace. It
// mirrors urlOnlyRe's intent (`^https?://\S+$` applied to
// TrimSpace input) but reads bytes directly so the per-long-line
// check does not need `string(line)` or a regex `MatchString`.
//
// The edge trim uses `bytes.TrimSpace` (allocation-free, sub-slice
// return) so the Unicode whitespace characters Go's `unicode.IsSpace`
// recognises — space, tab, newline, NBSP (U+00A0), and the rest of
// the Unicode whitespace block — are stripped exactly as
// `strings.TrimSpace(string(line))` would. Non-whitespace
// formatting characters such as zero-width joiner (U+200D) are
// not trimmed by either form.
//
// The internal-whitespace check stays ASCII-only — that matches
// Go's `regexp.\S` semantics for the original urlOnlyRe, where
// `\S` is the complement of the ASCII whitespace class
// `[\t\n\f\r ]`. A URL line whose body contains Unicode-only
// whitespace (e.g. an inner NBSP) is therefore treated as
// URL-only here, exactly as the regex treated it. This is
// behaviour-preserving against the old shape.
func isURLOnlyLine(line []byte) bool {
	line = bytes.TrimSpace(line)
	switch {
	case bytes.HasPrefix(line, urlPrefixHTTPS):
		line = line[len(urlPrefixHTTPS):]
	case bytes.HasPrefix(line, urlPrefixHTTP):
		line = line[len(urlPrefixHTTP):]
	default:
		return false
	}
	if len(line) == 0 {
		return false
	}
	for _, b := range line {
		if isASCIISpace(b) {
			return false
		}
	}
	return true
}

// isASCIISpace covers the ASCII whitespace bytes that interrupt a
// URL's `\S+` body in the original regex. Edge trimming uses
// bytes.TrimSpace (Unicode-aware) so this helper only fires on
// the inner-byte scan.
func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

var (
	urlPrefixHTTP  = []byte("http://")
	urlPrefixHTTPS = []byte("https://")
)

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	baseMax := r.Max
	if baseMax <= 0 {
		baseMax = 80
	}

	lc := r.buildCategories(f)

	// First pass: count byte-length candidates so the diagnostics
	// slice is allocated once. On diagnostic-heavy corpora the
	// append-growth re-copies dominated this rule's allocations; the
	// count is a length comparison per line against the smallest
	// possibly-active limit.
	minLimit := baseMax
	if r.HeadingMax != nil && *r.HeadingMax < minLimit {
		minLimit = *r.HeadingMax
	}
	if r.CodeBlockMax != nil && *r.CodeBlockMax < minLimit {
		minLimit = *r.CodeBlockMax
	}
	candidates := 0
	for _, line := range f.Lines {
		if len(line) > minLimit {
			candidates++
		}
	}
	if candidates == 0 {
		return nil
	}

	diags := make([]lint.Diagnostic, 0, candidates)
	for i, line := range f.Lines {
		lineNum := i + 1
		limit := r.activeMax(baseMax, lc, lineNum)

		// Fast path: byte length is always >= rune count, so if the
		// byte length fits, the rune count will too.
		if len(line) <= limit {
			continue
		}
		runeLen := utf8.RuneCount(line)
		if runeLen <= limit {
			continue
		}
		if r.isSkipped(line, lineNum, limit, lc) {
			continue
		}

		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     lineNum,
			Column:   limit + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  lineTooLongMessage(runeLen, limit),
		})
	}

	if len(diags) == 0 {
		return nil
	}
	return diags
}

// lineTooLongMessage returns the diagnostic message for a line of
// runeLen runes that exceeds limit. The synthetic engine bench
// produces ~60k diagnostics per iteration, all sharing the same
// (runeLen, limit) pair — pre-plan-195 each diagnostic paid a
// string concat + (when runeLen > 99) a strconv.Itoa allocation,
// totalling >1M alloc objects per 10-iteration run for this
// single line. The sync.Map keyed by (runeLen, limit) collapses
// repeats: real-world corpora rarely emit thousands of identical
// pairs, but when they do (e.g. a long boilerplate line repeated
// across files) the cache saves the allocation.
//
// The cache is bounded at lineTooLongCacheMaxEntries so a
// long-running process (the LSP server) does not accumulate one
// entry per distinct (runeLen, limit) pair seen over its
// lifetime. Past the cap, every call rebuilds the string and
// pays the original allocation — the worst-case cost is one
// string concat plus a map.Load, which matches the pre-plan-195
// shape and stays well below the per-rule alloc gate even when
// the cache is saturated.
func lineTooLongMessage(runeLen, limit int) string {
	key := lineTooLongKey{runeLen: runeLen, limit: limit}
	if v, ok := lineTooLongCache.Load(key); ok {
		return v.(string)
	}
	msg := "line too long (" + strconv.Itoa(runeLen) +
		" > " + strconv.Itoa(limit) + ")"
	if lineTooLongCacheCount.Load() < lineTooLongCacheMaxEntries {
		if _, loaded := lineTooLongCache.LoadOrStore(key, msg); !loaded {
			lineTooLongCacheCount.Add(1)
		}
	}
	return msg
}

// lineTooLongKey keys the message cache. A struct value with two
// int fields is comparable, so the sync.Map uses fast key equality
// without a string-build per lookup.
type lineTooLongKey struct {
	runeLen int
	limit   int
}

// lineTooLongCacheMaxEntries bounds the cache so a long-running
// process (the LSP server) cannot accumulate one entry per
// distinct (runeLen, limit) pair forever. 256 entries × ~24 bytes
// per string = ~6 KiB sustained, which covers the typical
// production spread (a handful of common limits — 80, 100, 120 —
// crossed with runeLens in the 80-200 range) without unbounded
// growth on pathological input.
const lineTooLongCacheMaxEntries = 256

var (
	lineTooLongCache      sync.Map
	lineTooLongCacheCount atomic.Int32
)

// hasSpacePastLimit returns true if line contains a space at or beyond
// the given limit column (0-indexed rune position).
func hasSpacePastLimit(line []byte, limit int) bool {
	i := 0
	for len(line) > 0 {
		r, size := utf8.DecodeRune(line)
		if i >= limit && r == ' ' {
			return true
		}
		line = line[size:]
		i++
	}
	return false
}

// collectTableLines returns a set of 1-based line numbers that are
// table rows. The scan replaces a per-line `tableLineRe.Match`
// (`^\s*\|`) with a tight byte loop; the regexp engine's internal
// buffer rentals were the dominant per-Check allocator for this
// helper before plan 195.
func collectTableLines(f *lint.File) map[int]struct{} {
	// Allocated lazily on the first table row: a nil map reads as
	// "no line is a table row", so table-free files skip the alloc.
	var lines map[int]struct{}
	for i, line := range f.Lines {
		if isTableLineStart(line) {
			if lines == nil {
				lines = make(map[int]struct{}, 8)
			}
			lines[i+1] = struct{}{}
		}
	}
	return lines
}

// isTableLineStart reports whether line opens with optional spaces
// or tabs followed by `|`. It is the allocation-free equivalent of
// `^\s*\|` for the subset of whitespace that mark a table-row leader
// in CommonMark; non-ASCII leading whitespace is not part of the
// CommonMark table grammar so the byte-level test is exact.
func isTableLineStart(line []byte) bool {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t':
			continue
		case '|':
			return true
		default:
			return false
		}
	}
	return false
}

// setextUnderlineRe matches a Setext heading underline (one or more = or -).
var setextUnderlineRe = regexp.MustCompile(`^[=-]+$`)

// collectHeadingLines returns a set of 1-based heading line numbers,
// including Setext underlines. On the flat Layer-0 path (plan 2606142147)
// it serves the classifier-derived set; otherwise it walks the AST. Both
// mark the ATX heading line, and for a Setext heading both the title line
// and its underline, so the two paths agree.
func collectHeadingLines(f *lint.File) map[int]struct{} {
	if hl, ok := lint.FlatHeadingLines(f); ok {
		return hl
	}
	lines := map[int]struct{}{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		ln := headingLineNum(h, f)
		if ln > 0 {
			lines[ln] = struct{}{}
			// For Setext headings, also include the underline line.
			if ln < len(f.Lines) {
				next := bytes.TrimSpace(f.Lines[ln]) // 0-indexed: ln is the next line
				if setextUnderlineRe.Match(next) {
					lines[ln+1] = struct{}{}
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return lines
}

// headingLineNum returns the 1-based line number of a heading node.
func headingLineNum(h *ast.Heading, f *lint.File) int {
	if h.Lines().Len() > 0 {
		return f.LineOfOffset(h.Lines().At(0).Start)
	}
	// ATX headings may have no Lines(); find line via child text nodes.
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
	}
	return 0
}

func isValidExclude(s string) bool {
	return s == "code-blocks" || s == "tables" || s == "urls"
}

var _ rule.Configurable = (*Rule)(nil)
