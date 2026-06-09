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
	msgRefImage        = "reference-style image; use inline form ![alt](url)"
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

// checkLinks walks the AST for *ast.Link and *ast.Image nodes that use
// reference-style syntax. Returns the diagnostic list and whether any
// reference-style link/image was found (so the unused-def pass can stay
// quiet when the link diagnostics already cover the file).
func (r *Rule) checkLinks(f *lint.File) ([]lint.Diagnostic, bool) {
	var diags []lint.Diagnostic
	hasRef := false

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var msg string
		switch v := n.(type) {
		case *ast.Link:
			if v.Reference == nil {
				return ast.WalkContinue, nil
			}
			msg = msgRefLink
		case *ast.Image:
			if v.Reference == nil {
				return ast.WalkContinue, nil
			}
			msg = msgRefImage
		default:
			return ast.WalkContinue, nil
		}
		hasRef = true
		line, col := nodePosition(n, f)
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   col,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  msg,
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
	defs := collectReferenceDefinitions(f)
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
	refs := scanFootnoteReferences(f, codeLines, codeSpans)
	defs := scanFootnoteDefinitions(f, codeLines)

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
		msg := footnotePlacementMessage(ref, defs, f.Lines)
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
	start int // byte offset of the definition line's start (incl. leading indentation); the Fix cut start
	end   int // byte offset just past the trailing newline
}

// collectReferenceDefinitions locates each reference definition in
// source so the rule can report a precise position. Reference
// definitions are consumed at parse time and never appear in the
// document AST, so their labels come from f.LinkReferences() — the
// canonical-parse reference set, already memoized on the *File — and
// no longer from a second goldmark parse (plan 188). The locate scan
// walks lines with scanRefDefLine, the byte-for-byte equivalent of
// the former refDefRE `(?m)^[ ]{0,3}\[([^\]\n]+)\]:[ \t]*\S+.*$`,
// the same scanner MDS053 already proved equal to that pattern.
// Lines inside fenced or indented code blocks are excluded so that
// definition-shaped lines in examples cannot produce false matches
// or corrupt Fix output.
func collectReferenceDefinitions(f *lint.File) []referenceDefinition {
	source := f.Source

	refs := f.LinkReferences()
	if len(refs) == 0 {
		return nil
	}

	codeLines := lint.CollectCodeBlockLines(f)
	var out []referenceDefinition

	lineNum := 1
	lineStart := 0
	for lineStart <= len(source) {
		eol := lineStart
		for eol < len(source) && source[eol] != '\n' {
			eol++
		}
		labelStart, labelEnd, ok := scanRefDefLine(source, lineStart, eol)
		if ok {
			raw := source[labelStart:labelEnd]
			// labelInRefs normalizes and scans, so check it first; the
			// codeLines membership is only consulted when the label is
			// actually referenced, keeping the && short-circuit the
			// previous `labelInRefs(...) && !codeLines[lineNum]` had.
			if labelInRefs(raw, refs) {
				if _, inCode := codeLines[lineNum]; !inCode {
					bracketAbs := labelStart - 1
					end := eol
					// Include the trailing newline so a fix can drop
					// the line cleanly.
					if end < len(source) && source[end] == '\n' {
						end++
					}
					out = append(out, referenceDefinition{
						label: string(raw),
						line:  lineNum,
						col:   f.ColumnOfOffset(bracketAbs),
						start: lineStart,
						end:   end,
					})
				}
			}
		}
		if eol >= len(source) {
			break
		}
		lineStart = eol + 1
		lineNum++
	}
	return out
}

// labelInRefs reports whether the normalized form of raw matches any
// label in refs (goldmark already normalized each ref's Label()).
// Mirrors the former `wanted[util.ToLinkReference(raw)]` membership
// test against ctx.References().
func labelInRefs(raw []byte, refs []lint.Reference) bool {
	normalized := util.ToLinkReference(raw)
	for _, ref := range refs {
		if stringEqualsBytes(normalized, ref.Label()) {
			return true
		}
	}
	return false
}

// stringEqualsBytes compares a string to a byte slice without
// allocating. Mirrors MDS053 (nounusedlinkdefinitions)'s
// alloc-free compare, keeping labelInRefs off the per-reference
// allocation hot path that `s == string(b)` creates for a
// variable s.
func stringEqualsBytes(s string, b []byte) bool {
	if len(s) != len(b) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != b[i] {
			return false
		}
	}
	return true
}

// scanRefDefLine examines source[lineStart:lineEnd] for the
// CommonMark reference definition pattern (0-3 leading spaces,
// `[label]:`, optional space/tab, a non-whitespace destination).
// Returns the absolute byte offsets of the bracket contents and
// ok=true on a hit. Mirrors the former regex
// `(?m)^[ ]{0,3}\[([^\]\n]+)\]:[ \t]*\S+.*$` byte-for-byte (the same
// scanner MDS053 uses). Used only for *locating* a definition after
// the parse already confirmed it exists.
func scanRefDefLine(source []byte, lineStart, lineEnd int) (labelStart, labelEnd int, ok bool) {
	j := lineStart
	spaces := 0
	for j < lineEnd && source[j] == ' ' && spaces < 3 {
		j++
		spaces++
	}
	if j >= lineEnd || source[j] != '[' {
		return -1, -1, false
	}
	labelStart = j + 1
	k := labelStart
	for k < lineEnd && source[k] != ']' {
		k++
	}
	if k >= lineEnd || k == labelStart {
		// Missing `]` on the line, or empty label (matches the
		// regex's `[^\]\n]+` which requires ≥ 1 char).
		return -1, -1, false
	}
	labelEnd = k
	colon := labelEnd + 1
	if colon >= lineEnd || source[colon] != ':' {
		return -1, -1, false
	}
	after := colon + 1
	for after < lineEnd && (source[after] == ' ' || source[after] == '\t') {
		after++
	}
	if after >= lineEnd {
		return -1, -1, false
	}
	// `\S` rejects ASCII whitespace; the trim loop above already
	// consumed ' ' and '\t', so this rejects \r and the other
	// whitespace bytes the regex form also rejected.
	if isASCIIWhitespace(source[after]) {
		return -1, -1, false
	}
	return labelStart, labelEnd, true
}

// isASCIIWhitespace mirrors Go regexp's `\s` (RE2): space, tab,
// newline, carriage return, and form feed. RE2's `\s` excludes the
// vertical tab (\v), so `\S` matches \v — the scanner must accept a
// \v-led destination to stay byte-identical with the old refDefRE.
func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
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
	f *lint.File, codeLines map[int]struct{}, codeSpans []byteRange,
) []footnoteOccurrence {
	source := f.Source
	matches := footnoteRefRE.FindAllSubmatchIndex(source, -1)
	var out []footnoteOccurrence
	for _, m := range matches {
		start := m[0]
		// Skip definitions: defRE is matched separately.
		if isFootnoteDefinitionAt(source, start) {
			continue
		}
		line := f.LineOfOffset(start)
		if _, ok := codeLines[line]; ok {
			continue
		}
		if rangeContains(codeSpans, start) {
			continue
		}
		out = append(out, footnoteOccurrence{
			slug:  string(source[m[2]:m[3]]),
			line:  line,
			col:   f.ColumnOfOffset(start),
			start: start,
			end:   m[1],
		})
	}
	return out
}

func scanFootnoteDefinitions(
	f *lint.File, codeLines map[int]struct{},
) []footnoteOccurrence {
	source := f.Source
	matches := footnoteDefRE.FindAllSubmatchIndex(source, -1)
	var out []footnoteOccurrence
	for _, m := range matches {
		start := m[0]
		line := f.LineOfOffset(start)
		if _, ok := codeLines[line]; ok {
			continue
		}
		out = append(out, footnoteOccurrence{
			slug:  string(source[m[2]:m[3]]),
			line:  line,
			col:   f.ColumnOfOffset(start),
			start: start,
			end:   m[1],
		})
	}
	return out
}

// isFootnoteDefinitionAt reports whether the `[^...]` token at offset
// `start` is a footnote definition rather than a reference. Definitions
// must begin a line with at most three leading spaces and be followed by
// `:` after the closing `]`. A mid-line `[^slug]:` is a reference, not
// a definition.
func isFootnoteDefinitionAt(source []byte, start int) bool {
	// bytes.LastIndexByte returns -1 when no '\n' precedes start (first
	// line), so lineStart correctly becomes 0.
	lineStart := bytes.LastIndexByte(source[:start], '\n') + 1
	indent := 0
	for i := lineStart; i < start; i++ {
		if source[i] != ' ' {
			return false
		}
		indent++
		if indent > 3 {
			return false
		}
	}
	close := bytes.IndexByte(source[start:], ']')
	if close < 0 {
		return false
	}
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
	lines [][]byte,
) string {
	defLines := map[int]struct{}{}
	hasMatchingSlug := false
	for _, d := range defs {
		defLines[d.line] = struct{}{}
		if d.slug == ref.slug {
			hasMatchingSlug = true
		}
	}
	endLine := paragraphEndLine(lines, ref.line, defLines)
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
// end of file. lines is f.Lines (pre-split by lint.NewFile).
func paragraphEndLine(lines [][]byte, line int, defLines map[int]struct{}) int {
	end := line
	for end < len(lines) && !isBlankLine(lines[end]) {
		if _, ok := defLines[end+1]; ok {
			break
		}
		end++
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
func nodePosition(n ast.Node, f *lint.File) (int, int) {
	source := f.Source
	seg := firstDescendantText(n)
	start := seg.Start
	for start > 0 && source[start-1] != '\n' && source[start-1] != '[' {
		start--
	}
	if start > 0 && source[start-1] == '[' {
		start--
	}
	return f.LineOfOffset(start), f.ColumnOfOffset(start)
}

func firstDescendantText(n ast.Node) text.Segment {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return t.Segment
		}
		if seg := firstDescendantText(c); seg != (text.Segment{}) {
			return seg
		}
	}
	return text.Segment{}
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
	defCuts := collectDefinitionCuts(f, usedLabels)
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
		var ref *ast.ReferenceLink
		var dest, title []byte
		isImage := false
		switch v := n.(type) {
		case *ast.Link:
			if v.Reference == nil {
				return ast.WalkContinue, nil
			}
			ref = v.Reference
			dest, title = v.Destination, v.Title
		case *ast.Image:
			if v.Reference == nil {
				return ast.WalkContinue, nil
			}
			ref = v.Reference
			dest, title = v.Destination, v.Title
			isImage = true
		default:
			return ast.WalkContinue, nil
		}
		start, end, txt, ok := linkSourceSpan(n, f.Source)
		if !ok {
			// Cannot recover the source span (e.g. empty-text link/image).
			// Leave the link/image and its definition untouched so we never
			// emit a malformed rewrite.
			return ast.WalkContinue, nil
		}
		// For images `![alt][id]`, the `!` sits just before `[`.
		if isImage && start > 0 && f.Source[start-1] == '!' {
			start--
		}
		usedLabels[util.ToLinkReference(ref.Value)] = true
		cuts = append(cuts, fixCut{
			start: start,
			end:   end,
			repl:  buildInlineMedia(txt, dest, title, isImage),
		})
		return ast.WalkContinue, nil
	})
	return cuts, usedLabels
}

func collectDefinitionCuts(f *lint.File, usedLabels map[string]bool) []fixCut {
	if len(usedLabels) == 0 {
		return nil
	}
	source := f.Source
	defs := collectReferenceDefinitions(f)
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

// linkSourceSpan returns the byte span of an entire link/image expression
// (`[text](...)` or `[text][id]` etc.) and the inner text. For
// reference links the closing bracket is followed by either nothing
// (shortcut), `[]` (collapsed), or `[id]` (full). The third return
// is false when the link has no text descendants — an empty-text
// link like `[][id]` — in which case the source span cannot be
// recovered from the AST and the caller should skip the rewrite.
// Note: for images the returned start points to `[`, not `!`; the
// caller is responsible for extending start by one to include `!`.
func linkSourceSpan(n ast.Node, source []byte) (int, int, string, bool) {
	seg := firstDescendantText(n)
	if seg == (text.Segment{}) {
		return 0, 0, "", false
	}
	textStart := seg.Start
	for textStart > 0 && source[textStart-1] != '\n' && source[textStart-1] != '[' {
		textStart--
	}
	if textStart == 0 || source[textStart-1] != '[' {
		return 0, 0, "", false
	}
	openBracket := textStart - 1
	textEnd := findClosingBracket(source, textStart)
	end := skipReferenceLabel(source, textEnd+1)
	return openBracket, end, string(source[textStart:textEnd]), true
}

// findClosingBracket scans from `pos` for the `]` that balances the
// opening `[` immediately before `pos`, honoring backslash escapes
// and nested brackets. Goldmark accepts nested brackets in link text
// when an inline link is embedded — for example, `[a [b](x)][id]` —
// so a depth counter is required to identify the outer `]`.
func findClosingBracket(source []byte, pos int) int {
	depth := 1
	for ; pos < len(source); pos++ {
		switch source[pos] {
		case '\\':
			pos++
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return pos
			}
		}
	}
	return pos
}

// skipReferenceLabel advances past optional whitespace and any
// trailing `[label]` (full reference) or `[]` (collapsed reference).
// A shortcut reference has nothing after the link text.
func skipReferenceLabel(source []byte, end int) int {
	scan := end
	for scan < len(source) && (source[scan] == ' ' || source[scan] == '\t') {
		scan++
	}
	if scan < len(source) && source[scan] == '[' {
		if closeIdx := bytes.IndexByte(source[scan:], ']'); closeIdx >= 0 {
			return scan + closeIdx + 1
		}
	}
	return end
}

// buildInlineMedia renders `[text](dest "title")` for links or
// `![text](dest "title")` for images.
func buildInlineMedia(text string, dest, title []byte, image bool) []byte {
	var b bytes.Buffer
	if image {
		b.WriteByte('!')
	}
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

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Inline reference-style links" }
