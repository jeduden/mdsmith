// Package noreferencestyle implements MDS043, which forbids
// reference-style links and footnotes. These constructs require global
// definition resolution, moving Markdown from a context-free to a
// context-sensitive grammar.
package noreferencestyle

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func init() {
	rule.Register(&Rule{})
}

// Rule forbids reference-style links and footnotes.
type Rule struct {
	// AllowFootnotes opts back into footnotes. Numeric slugs and
	// definitions placed away from the referencing paragraph are still
	// rejected.
	AllowFootnotes bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS043" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-reference-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

const (
	msgRefLink         = "reference-style link; use inline form [text](url)"
	msgFootnote        = "footnote reference; footnotes are not allowed"
	msgFootnoteNum     = "footnote slug is numeric; use a meaningful slug"
	msgFootnoteMissing = "footnote reference has no matching definition"
	msgFootnotePlace   = "footnote definition must follow its referencing paragraph"
	msgUnusedDef       = "unused reference definition: [%s]"
)

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	diags, hasRefLinks := r.checkLinks(f)
	diags = append(diags, r.checkUnusedDefinitions(f, hasRefLinks)...)
	diags = append(diags, r.checkFootnotes(f)...)

	return diags
}

// checkLinks walks the AST for *ast.Link nodes. Returns the diagnostic
// list and whether any reference-style links were found (so the
// unused-def pass can stay quiet when the link diagnostics already
// cover the file).
func (r *Rule) checkLinks(f *lint.File) ([]lint.Diagnostic, bool) {
	var diags []lint.Diagnostic
	hasRef := false

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := n.(*ast.Link)
		if !ok || link.Reference == nil {
			return ast.WalkContinue, nil
		}
		hasRef = true
		line, col := nodePosition(link, f.Source)
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   col,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  msgRefLink,
		})
		return ast.WalkContinue, nil
	})

	return diags, hasRef
}

// checkUnusedDefinitions emits a diagnostic for each reference
// definition whose label is not used by any reference-style link.
// When the file contains reference-style links, the link diagnostics
// already cover the issue and definitions are left alone.
func (r *Rule) checkUnusedDefinitions(
	f *lint.File, hasRefLinks bool,
) []lint.Diagnostic {
	if hasRefLinks {
		return nil
	}
	defs := collectReferenceDefinitions(f.Source)
	var diags []lint.Diagnostic
	for _, d := range defs {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     d.line,
			Column:   d.col,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf(msgUnusedDef, d.label),
		})
	}
	return diags
}

// checkFootnotes scans source bytes for footnote references and
// definitions. The default lint parser does not enable goldmark's
// footnote extension, so the AST does not surface footnote nodes —
// regex over source bytes (filtered against code-block ranges) is
// sufficient and avoids reparsing the file.
func (r *Rule) checkFootnotes(f *lint.File) []lint.Diagnostic {
	codeLines := lint.CollectCodeBlockLines(f)
	codeSpans := collectCodeSpanRanges(f.AST, f.Source)
	refs := scanFootnoteReferences(f.Source, codeLines, codeSpans)
	defs := scanFootnoteDefinitions(f.Source, codeLines)

	var diags []lint.Diagnostic
	for _, ref := range refs {
		if !r.AllowFootnotes {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     ref.line,
				Column:   ref.col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msgFootnote,
			})
			continue
		}
		if isNumericSlug(ref.slug) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     ref.line,
				Column:   ref.col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msgFootnoteNum,
			})
			continue
		}
		msg := footnotePlacementMessage(ref, defs, f.Source)
		if msg != "" {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     ref.line,
				Column:   ref.col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msg,
			})
		}
	}
	return diags
}

// referenceDefinition records one `[label]: dest` line in source.
type referenceDefinition struct {
	label string
	line  int
	col   int
	start int // byte offset of the `[` opening the label
	end   int // byte offset just past the trailing newline
}

// collectReferenceDefinitions re-parses the source with goldmark to
// pick up reference definitions (which are consumed at parse time and
// never appear in the document AST), then locates each in source so
// the rule can report a precise position.
func collectReferenceDefinitions(source []byte) []referenceDefinition {
	ctx := parser.NewContext()
	lint.NewParser().Parse(text.NewReader(source), parser.WithContext(ctx))

	refs := ctx.References()
	if len(refs) == 0 {
		return nil
	}

	var out []referenceDefinition
	usedSpans := map[int]bool{}
	for _, ref := range refs {
		def, ok := findReferenceDefinitionInSource(
			source, string(ref.Label()), usedSpans,
		)
		if !ok {
			continue
		}
		usedSpans[def.start] = true
		out = append(out, def)
	}
	return out
}

// refDefRE matches a CommonMark reference definition at the start of
// a line: optional 0-3 spaces, [label]: dest (with optional title).
// Used only for *locating* a definition after goldmark already
// confirmed it exists, so a permissive regex is safe.
var refDefRE = regexp.MustCompile(`(?m)^[ ]{0,3}\[([^\]\n]+)\]:[ \t]*\S+.*$`)

// findReferenceDefinitionInSource locates a definition whose
// normalised label matches `label`. The `seen` set prevents the same
// source line from matching two definitions in the rare case the
// parser stored duplicate labels.
func findReferenceDefinitionInSource(
	source []byte, label string, seen map[int]bool,
) (referenceDefinition, bool) {
	matches := refDefRE.FindAllSubmatchIndex(source, -1)
	for _, m := range matches {
		start := m[0]
		if seen[start] {
			continue
		}
		raw := source[m[2]:m[3]]
		if util.ToLinkReference(raw) != label {
			continue
		}
		end := m[1]
		// Include the trailing newline so a fix can drop the line cleanly.
		if end < len(source) && source[end] == '\n' {
			end++
		}
		bracketAbs := m[2] - 1 // m[2] starts the label, so `[` is the byte before
		line := lineOfOffset(source, bracketAbs)
		col := columnOfOffset(source, bracketAbs)
		return referenceDefinition{
			label: string(raw),
			line:  line,
			col:   col,
			start: start,
			end:   end,
		}, true
	}
	return referenceDefinition{}, false
}

// footnoteOccurrence records one `[^slug]` reference in source.
type footnoteOccurrence struct {
	slug  string
	line  int
	col   int
	start int
	end   int
}

// footnoteRefRE matches a footnote-style token `[^slug]`. Whether
// the token is a reference vs a definition is decided afterwards by
// isFootnoteDefinitionAt — keeping the regex narrow ensures adjacent
// references like `[^a][^b]` are both detected (an alternation that
// consumed the trailing byte would swallow the `[` of the second).
var footnoteRefRE = regexp.MustCompile(`\[\^([^\]\n]+)\]`)

// footnoteDefRE matches a footnote definition line: optional indent,
// `[^slug]:` then any text.
var footnoteDefRE = regexp.MustCompile(`(?m)^[ ]{0,3}\[\^([^\]\n]+)\]:`)

func scanFootnoteReferences(
	source []byte, codeLines map[int]bool, codeSpans []byteRange,
) []footnoteOccurrence {
	matches := footnoteRefRE.FindAllSubmatchIndex(source, -1)
	var out []footnoteOccurrence
	for _, m := range matches {
		start := m[0]
		// Skip definitions: defRE is matched separately.
		if isFootnoteDefinitionAt(source, start) {
			continue
		}
		line := lineOfOffset(source, start)
		if codeLines[line] {
			continue
		}
		if rangeContains(codeSpans, start) {
			continue
		}
		out = append(out, footnoteOccurrence{
			slug:  string(source[m[2]:m[3]]),
			line:  line,
			col:   columnOfOffset(source, start),
			start: start,
			end:   m[1],
		})
	}
	return out
}

func scanFootnoteDefinitions(
	source []byte, codeLines map[int]bool,
) []footnoteOccurrence {
	matches := footnoteDefRE.FindAllSubmatchIndex(source, -1)
	var out []footnoteOccurrence
	for _, m := range matches {
		start := m[0]
		line := lineOfOffset(source, start)
		if codeLines[line] {
			continue
		}
		out = append(out, footnoteOccurrence{
			slug:  string(source[m[2]:m[3]]),
			line:  line,
			col:   columnOfOffset(source, start),
			start: start,
			end:   m[1],
		})
	}
	return out
}

// isFootnoteDefinitionAt reports whether the `[^...]` token at offset
// `start` is followed by `:` after the closing `]`, making it a
// definition rather than a reference. The caller has already matched
// `[^...]` at `start`, so `]` is guaranteed to appear later.
func isFootnoteDefinitionAt(source []byte, start int) bool {
	close := bytes.IndexByte(source[start:], ']')
	pos := start + close + 1
	return pos < len(source) && source[pos] == ':'
}

// footnotePlacementMessage returns the empty string when `ref` has a
// matching definition immediately after its paragraph. Otherwise it
// returns a diagnostic message that distinguishes "no matching
// definition" from "definition exists but is misplaced". A single
// blank line separator is allowed (matching the typical footnote-
// block style).
func footnotePlacementMessage(
	ref footnoteOccurrence,
	defs []footnoteOccurrence,
	source []byte,
) string {
	defLines := map[int]bool{}
	hasMatchingSlug := false
	for _, d := range defs {
		defLines[d.line] = true
		if d.slug == ref.slug {
			hasMatchingSlug = true
		}
	}
	endLine := paragraphEndLine(source, ref.line, defLines)
	for _, d := range defs {
		if d.slug != ref.slug {
			continue
		}
		if d.line == endLine+1 || d.line == endLine+2 {
			return ""
		}
	}
	if !hasMatchingSlug {
		return msgFootnoteMissing
	}
	return msgFootnotePlace
}

// paragraphEndLine returns the 1-based line number of the last line
// belonging to the paragraph that contains `line`. The paragraph
// stops at the next blank line, the next footnote definition, or
// end of file.
func paragraphEndLine(source []byte, line int, defLines map[int]bool) int {
	lines := bytes.Split(source, []byte("\n"))
	end := line
	for end < len(lines) {
		next := end + 1
		if next > len(lines) {
			break
		}
		if isBlankLine(lines[next-1]) {
			break
		}
		if defLines[next] {
			break
		}
		end = next
	}
	return end
}

func isBlankLine(line []byte) bool {
	for _, b := range line {
		if b != ' ' && b != '\t' {
			return false
		}
	}
	return true
}

func isNumericSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// byteRange is an inclusive [start, end) byte range in source.
type byteRange struct {
	start, end int
}

func rangeContains(ranges []byteRange, off int) bool {
	for _, r := range ranges {
		if off >= r.start && off < r.end {
			return true
		}
	}
	return false
}

// collectCodeSpanRanges returns the byte ranges of inline code spans
// in the document. Footnote-shaped tokens inside backticks are not
// real footnote references.
func collectCodeSpanRanges(root ast.Node, source []byte) []byteRange {
	var out []byteRange
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		span, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		seg := firstSegment(span)
		last := lastSegment(span)
		if seg.Start == 0 && seg.Stop == 0 {
			return ast.WalkContinue, nil
		}
		// Extend backwards to include opening backticks; extend
		// forwards across closing backticks.
		start := seg.Start
		for start > 0 && source[start-1] == '`' {
			start--
		}
		end := last.Stop
		for end < len(source) && source[end] == '`' {
			end++
		}
		out = append(out, byteRange{start: start, end: end})
		return ast.WalkContinue, nil
	})
	return out
}

func firstSegment(n ast.Node) text.Segment {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return t.Segment
		}
	}
	return text.Segment{}
}

func lastSegment(n ast.Node) text.Segment {
	var seg text.Segment
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			seg = t.Segment
		}
	}
	return seg
}

// nodePosition returns a 1-based (line, column) for the source
// position of `n`. For inline link nodes goldmark records the inner
// text segment, so we walk back from the first text child to the
// opening `[`.
func nodePosition(n ast.Node, source []byte) (int, int) {
	seg := firstDescendantText(n)
	if seg.Start == 0 && seg.Stop == 0 {
		return 1, 1
	}
	start := seg.Start
	// Walk back to the most recent `[` on the same line.
	for start > 0 && source[start-1] != '\n' && source[start-1] != '[' {
		start--
	}
	if start > 0 && source[start-1] == '[' {
		start--
	}
	return lineOfOffset(source, start), columnOfOffset(source, start)
}

func firstDescendantText(n ast.Node) text.Segment {
	var seg text.Segment
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := c.(*ast.Text); ok {
			seg = t.Segment
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return seg
}

func lineOfOffset(source []byte, off int) int {
	line := 1
	for i := 0; i < off; i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

func columnOfOffset(source []byte, off int) int {
	start := off
	for start > 0 && source[start-1] != '\n' {
		start--
	}
	return off - start + 1
}

// fixCut is a single byte-range replacement in source. `repl` may be
// nil (pure removal) or hold the rewritten text for that span.
type fixCut struct {
	start, end int
	repl       []byte
}

// Fix implements rule.FixableRule. It rewrites every reference-style
// link to its inline equivalent and drops the matching reference
// definitions. Footnotes are not auto-fixed.
func (r *Rule) Fix(f *lint.File) []byte {
	linkCuts, usedLabels := collectLinkRewrites(f)
	defCuts := collectDefinitionCuts(f.Source, usedLabels)
	cuts := append(linkCuts, defCuts...)
	if len(cuts) == 0 {
		out := make([]byte, len(f.Source))
		copy(out, f.Source)
		return out
	}
	return applyCuts(f.Source, cuts)
}

func collectLinkRewrites(f *lint.File) ([]fixCut, map[string]bool) {
	var cuts []fixCut
	usedLabels := map[string]bool{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := n.(*ast.Link)
		if !ok || link.Reference == nil {
			return ast.WalkContinue, nil
		}
		start, end, txt, ok := linkSourceSpan(link, f.Source)
		if !ok {
			return ast.WalkContinue, nil
		}
		usedLabels[util.ToLinkReference(link.Reference.Value)] = true
		cuts = append(cuts, fixCut{
			start: start,
			end:   end,
			repl:  buildInlineLink(txt, link.Destination, link.Title),
		})
		return ast.WalkContinue, nil
	})
	return cuts, usedLabels
}

func collectDefinitionCuts(source []byte, usedLabels map[string]bool) []fixCut {
	defs := collectReferenceDefinitions(source)
	var cuts []fixCut
	for _, d := range defs {
		if !usedLabels[util.ToLinkReference([]byte(d.label))] {
			continue
		}
		start := d.start
		// Consume the blank line before the definition so removal
		// doesn't leave back-to-back newlines at end of file.
		if start >= 2 && source[start-1] == '\n' && source[start-2] == '\n' {
			start--
		}
		cuts = append(cuts, fixCut{start: start, end: d.end, repl: nil})
	}
	return cuts
}

func applyCuts(source []byte, cuts []fixCut) []byte {
	sort.Slice(cuts, func(i, j int) bool {
		return cuts[i].start < cuts[j].start
	})
	var out bytes.Buffer
	prev := 0
	for _, c := range cuts {
		if c.start < prev {
			continue
		}
		out.Write(source[prev:c.start])
		out.Write(c.repl)
		prev = c.end
	}
	out.Write(source[prev:])
	return out.Bytes()
}

// linkSourceSpan returns the byte span of an entire link expression
// (`[text](...)` or `[text][id]` etc.), and the inner text. For
// reference links the closing bracket is followed by either nothing
// (shortcut), `[]` (collapsed) or `[id]` (full).
func linkSourceSpan(link *ast.Link, source []byte) (int, int, string, bool) {
	seg := firstDescendantText(link)
	if seg.Start == 0 && seg.Stop == 0 {
		return 0, 0, "", false
	}
	// Extend backward to opening `[`.
	start := seg.Start
	for start > 0 && source[start-1] != '\n' && source[start-1] != '[' {
		start--
	}
	if start == 0 || source[start-1] != '[' {
		return 0, 0, "", false
	}
	openBracket := start - 1
	// Find matching closing `]` for the link text. Allow nested
	// brackets only via simple depth tracking — sufficient for
	// CommonMark link text which forbids unescaped `[` inside.
	pos := start
	depth := 1
	for pos < len(source) && depth > 0 {
		switch source[pos] {
		case '\\':
			pos++
		case '[':
			depth++
		case ']':
			depth--
		}
		if depth == 0 {
			break
		}
		pos++
	}
	if pos >= len(source) {
		return 0, 0, "", false
	}
	textEnd := pos // offset of the closing `]`
	innerText := string(source[start:textEnd])
	end := textEnd + 1 // offset just past the closing `]`

	// Skip optional whitespace then a `[label]` for full or `[]` for
	// collapsed reference. A shortcut reference has nothing after.
	scan := end
	for scan < len(source) && (source[scan] == ' ' || source[scan] == '\t') {
		scan++
	}
	if scan < len(source) && source[scan] == '[' {
		closeIdx := bytes.IndexByte(source[scan:], ']')
		if closeIdx >= 0 {
			end = scan + closeIdx + 1
		}
	}

	return openBracket, end, innerText, true
}

// buildInlineLink renders `[text](dest "title")`.
func buildInlineLink(text string, dest, title []byte) []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	b.WriteString(text)
	b.WriteByte(']')
	b.WriteByte('(')
	b.Write(dest)
	if len(title) > 0 {
		b.WriteString(` "`)
		b.WriteString(strings.ReplaceAll(string(title), `"`, `\"`))
		b.WriteByte('"')
	}
	b.WriteByte(')')
	return b.Bytes()
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "allow-footnotes":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf(
					"no-reference-style: allow-footnotes must be a bool, got %T", v,
				)
			}
			r.AllowFootnotes = b
		default:
			return fmt.Errorf("no-reference-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"allow-footnotes": false,
	}
}

var (
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
