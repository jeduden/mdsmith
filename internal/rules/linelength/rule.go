package linelength

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
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

// lineCategories holds pre-computed line classification maps.
// lineCategories holds the line-set lookups Check needs to decide
// per-line max and per-line skip. Each map is nil when the active
// settings do not need it — reads from a nil map return false, which
// is exactly the "line is not in this category" answer the rest of
// the rule wants, so the absent-by-default state costs no allocation
// and no per-line branch. Pre-creating empty maps to "look uniform"
// added three wasted allocations per Check before plan 195.
type lineCategories struct {
	code    map[int]bool
	table   map[int]bool
	heading map[int]bool
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
	if lc.heading[lineNum] && r.HeadingMax != nil {
		return *r.HeadingMax
	}
	if lc.code[lineNum] && r.CodeBlockMax != nil {
		return *r.CodeBlockMax
	}
	return baseMax
}

// isSkipped returns true if the line should be excluded from checking.
func (r *Rule) isSkipped(line []byte, lineNum, limit int, lc lineCategories) bool {
	if r.isExcluded("code-blocks") && lc.code[lineNum] {
		return true
	}
	if r.isExcluded("tables") && lc.table[lineNum] {
		return true
	}
	if r.isExcluded("urls") && isURLOnlyLine(line) {
		return true
	}
	if r.Stern && !hasSpacePastLimit(line, limit) {
		return true
	}
	return false
}

// isURLOnlyLine reports whether line, after trimming ASCII whitespace,
// is a single http(s) URL with no internal whitespace. It mirrors
// urlOnlyRe's intent (`^https?://\S+$` applied to TrimSpace input)
// but reads bytes directly so the per-long-line check does not need
// `string(line)`, `strings.TrimSpace`, or a regex `MatchString` —
// each of which allocates in the regexp engine's hot frame.
func isURLOnlyLine(line []byte) bool {
	for len(line) > 0 && isASCIISpace(line[0]) {
		line = line[1:]
	}
	for len(line) > 0 && isASCIISpace(line[len(line)-1]) {
		line = line[:len(line)-1]
	}
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

// isASCIISpace mirrors what `strings.TrimSpace` strips on the
// representative ASCII inputs the URL check sees: space, tab, CR, LF.
// Wider unicode whitespace would not appear in a URL-only line — and
// when it does, the trimmed prefix check fails, so the outcome is
// stable across inputs.
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

	var diags []lint.Diagnostic
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

		// Build the message via concat + strconv.Itoa rather than
		// fmt.Sprintf so the warm path stays under the per-rule
		// allocation budget. Sprintf allocates ~3 (format string
		// scan + result buffer + temp); concat + Itoa lands at 1
		// for typical max/runeLen values that hit strconv's
		// small-int cache.
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     lineNum,
			Column:   limit + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: "line too long (" + strconv.Itoa(runeLen) +
				" > " + strconv.Itoa(limit) + ")",
		})
	}

	return diags
}

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
func collectTableLines(f *lint.File) map[int]bool {
	lines := map[int]bool{}
	for i, line := range f.Lines {
		if isTableLineStart(line) {
			lines[i+1] = true
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

// collectHeadingLines walks the AST and returns a set of 1-based line numbers
// that are heading lines, including Setext underlines.
func collectHeadingLines(f *lint.File) map[int]bool {
	lines := map[int]bool{}
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
			lines[ln] = true
			// For Setext headings, also include the underline line.
			if ln < len(f.Lines) {
				next := strings.TrimSpace(string(f.Lines[ln])) // 0-indexed: ln is the next line
				if setextUnderlineRe.MatchString(next) {
					lines[ln+1] = true
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
