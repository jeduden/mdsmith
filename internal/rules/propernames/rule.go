// Package propernames implements MDS050, which checks that proper names
// (e.g. JavaScript, GitHub) appear with their configured casing.
package propernames

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

func init() {
	rule.Register(&Rule{})
}

// Rule reports occurrences of configured proper names that do not match
// their canonical casing (e.g. "Javascript" when "JavaScript" is configured).
type Rule struct {
	// Names is the list of proper names with their canonical casing.
	// The names list appends across config layers so kind layers extend
	// rather than replace the inherited vocabulary (same convention as
	// placeholders:).
	Names []string
	// CheckCode enables checking inside code spans and code blocks.
	CheckCode bool
	// CheckHTML enables checking inside raw HTML and HTML blocks.
	CheckHTML bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS050" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "proper-names" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// isWordChar reports whether b is an ASCII letter, digit, or underscore.
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// asciiToLower lowercases ASCII uppercase letters only. Unlike bytes.ToLower,
// this never changes the byte length of the slice, which keeps byte offsets
// stable when matching lowerText positions back to the original text.
func asciiToLower(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + ('a' - 'A')
		} else {
			out[i] = c
		}
	}
	return out
}

// wrongMatch holds one wrong-cased occurrence.
type wrongMatch struct {
	start  int // byte offset in f.Source
	length int
	name   string
}

// nameEntry holds the precomputed byte representations of one configured name.
// Built once per Check/Fix call and reused across all text segments.
type nameEntry struct {
	canonical []byte // original spelling, e.g. []byte("JavaScript")
	lower     []byte // ASCII-lowercased, e.g. []byte("javascript")
	str       string // canonical as string, used in diagnostics
}

// buildNameEntries precomputes nameEntry values for all configured names.
func (r *Rule) buildNameEntries() []nameEntry {
	entries := make([]nameEntry, 0, len(r.Names))
	for _, name := range r.Names {
		if len(name) == 0 {
			continue
		}
		b := []byte(name)
		entries = append(entries, nameEntry{
			canonical: b,
			lower:     asciiToLower(b),
			str:       name,
		})
	}
	return entries
}

// scanBytes finds all wrong-cased occurrences of entries within the text
// slice, which starts at baseOffset in the full source. source is the full
// file source (used for left-boundary checks before the segment start).
func scanBytes(entries []nameEntry, text []byte, baseOffset int, source []byte) []wrongMatch {
	if len(entries) == 0 || len(text) == 0 {
		return nil
	}
	// Lowercase the segment once; reused for every configured name.
	// asciiToLower never changes byte length, keeping offsets stable.
	lowerText := asciiToLower(text)
	var results []wrongMatch
	for _, e := range entries {
		n := len(e.canonical)
		if n > len(text) {
			continue
		}
		for i := 0; i <= len(lowerText)-n; i++ {
			// Left boundary: the byte before the match (in source) must not
			// be a word character, or the match is at the start of the source.
			absOffset := baseOffset + i
			if absOffset > 0 && isWordChar(source[absOffset-1]) {
				continue
			}
			// Case-insensitive prefix match.
			if !bytes.Equal(lowerText[i:i+n], e.lower) {
				continue
			}
			// Skip if casing already matches the canonical spelling.
			if bytes.Equal(text[i:i+n], e.canonical) {
				continue
			}
			results = append(results, wrongMatch{
				start:  absOffset,
				length: n,
				name:   e.str,
			})
		}
	}
	return results
}

// lineNode is implemented by AST block nodes that store their content
// as a list of text segments (FencedCodeBlock, CodeBlock, HTMLBlock).
type lineNode interface {
	Lines() *text.Segments
}

// scanLines scans all lines from a block node (FencedCodeBlock, CodeBlock,
// HTMLBlock) and appends any wrong-cased matches to acc, returning it.
func scanLines(entries []nameEntry, n lineNode, f *lint.File, acc []wrongMatch) []wrongMatch {
	segs := n.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		acc = append(acc, scanBytes(entries, seg.Value(f.Source), seg.Start, f.Source)...)
	}
	return acc
}

// scanCodeSpanChildren scans the Text children of a CodeSpan node and appends
// any wrong-cased matches to acc, returning it.
func scanCodeSpanChildren(entries []nameEntry, v *ast.CodeSpan, f *lint.File, acc []wrongMatch) []wrongMatch {
	for c := v.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		seg := t.Segment
		acc = append(acc, scanBytes(entries, seg.Value(f.Source), seg.Start, f.Source)...)
	}
	return acc
}

// scanRawHTMLSegments scans the Segments of a RawHTML node and appends any
// wrong-cased matches to acc, returning it.
func scanRawHTMLSegments(entries []nameEntry, v *ast.RawHTML, f *lint.File, acc []wrongMatch) []wrongMatch {
	for i := 0; i < v.Segments.Len(); i++ {
		seg := v.Segments.At(i)
		acc = append(acc, scanBytes(entries, seg.Value(f.Source), seg.Start, f.Source)...)
	}
	return acc
}

// collectMatches walks the AST and gathers all wrong-cased matches.
// nameEntry values are precomputed once here and reused across all segments.
func (r *Rule) collectMatches(f *lint.File) []wrongMatch {
	entries := r.buildNameEntries()
	if len(entries) == 0 {
		return nil
	}
	var all []wrongMatch

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch v := n.(type) {
		case *ast.AutoLink:
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			if r.CheckCode {
				all = scanCodeSpanChildren(entries, v, f, all)
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			if r.CheckCode {
				all = scanLines(entries, n, f, all)
			}
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock:
			if r.CheckHTML {
				all = scanLines(entries, n, f, all)
			}
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			if r.CheckHTML {
				all = scanRawHTMLSegments(entries, v, f, all)
			}
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			seg := v.Segment
			all = append(all, scanBytes(entries, seg.Value(f.Source), seg.Start, f.Source)...)
		}

		return ast.WalkContinue, nil
	})

	return all
}

// normalizeMatches sorts matches by start offset (ties broken by longest
// match first) and removes overlapping entries, keeping the longest match
// at each offset. Both Check and Fix call this so they agree on which
// occurrences constitute a single diagnostic/replacement.
func normalizeMatches(matches []wrongMatch) []wrongMatch {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		return matches[i].length > matches[j].length
	})
	out := matches[:0]
	prev := 0
	for _, m := range matches {
		if m.start < prev {
			continue
		}
		out = append(out, m)
		prev = m.start + m.length
	}
	return out
}

// collectMatchesInline gathers wrong-cased matches on the parse-skipped path
// (f.AST nil). It re-parses each inline run on demand (lint.InlineBlocks) and
// applies the same node switch collectMatches applies on the tree, mapping
// each run-local segment offset back to the document with the run base so the
// scan reads f.Source at the same absolute positions. Code blocks and HTML
// blocks carry no inline markup and are excluded from the runs, so they are
// scanned separately from the Layer 0 block spans when check-code / check-html
// is enabled — together reproducing the AST walk's match set.
func (r *Rule) collectMatchesInline(f *lint.File) []wrongMatch {
	entries := r.buildNameEntries()
	if len(entries) == 0 {
		return nil
	}
	var all []wrongMatch
	for _, blk := range lint.InlineBlocks(f) {
		base := blk.Offset
		_ = ast.Walk(blk.Node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch v := n.(type) {
			case *ast.AutoLink:
				return ast.WalkSkipChildren, nil
			case *ast.CodeSpan:
				if r.CheckCode {
					all = scanCodeSpanChildrenBase(entries, v, f, base, all)
				}
				return ast.WalkSkipChildren, nil
			case *ast.RawHTML:
				if r.CheckHTML {
					all = scanRawHTMLSegmentsBase(entries, v, f, base, all)
				}
				return ast.WalkSkipChildren, nil
			case *ast.Text:
				all = scanTextSegmentBase(entries, v.Segment, f, base, all)
			}
			return ast.WalkContinue, nil
		})
	}
	if r.CheckCode || r.CheckHTML {
		all = r.scanBlockSpans(entries, f, all)
	}
	return all
}

// scanTextSegmentBase scans one run-local text segment at document position
// base+seg.Start, appending matches to acc.
func scanTextSegmentBase(entries []nameEntry, seg text.Segment, f *lint.File, base int, acc []wrongMatch) []wrongMatch {
	abs := base + seg.Start
	return append(acc, scanBytes(entries, f.Source[abs:base+seg.Stop], abs, f.Source)...)
}

// scanCodeSpanChildrenBase is scanCodeSpanChildren with a run base added to
// each child's segment offset.
func scanCodeSpanChildrenBase(entries []nameEntry, v *ast.CodeSpan, f *lint.File, base int, acc []wrongMatch) []wrongMatch {
	for c := v.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		acc = scanTextSegmentBase(entries, t.Segment, f, base, acc)
	}
	return acc
}

// scanRawHTMLSegmentsBase is scanRawHTMLSegments with a run base added to each
// segment offset.
func scanRawHTMLSegmentsBase(entries []nameEntry, v *ast.RawHTML, f *lint.File, base int, acc []wrongMatch) []wrongMatch {
	for i := 0; i < v.Segments.Len(); i++ {
		acc = scanTextSegmentBase(entries, v.Segments.At(i), f, base, acc)
	}
	return acc
}

// scanBlockSpans scans the body lines of the Layer 0 code and HTML block spans
// that the inline runs exclude, so the parse-skipped path covers the same
// FencedCodeBlock / CodeBlock / HTMLBlock content the AST walk's scanLines
// covers. The body line range and per-line content offset mirror goldmark's
// segment bounds: a fenced block excludes its fence lines and strips the
// fence's leading indent, an indented block strips its four-space / tab
// indent, and an HTML block keeps its lines verbatim — so each match lands at
// the same document offset (and thus the same column) the AST path reports.
func (r *Rule) scanBlockSpans(entries []nameEntry, f *lint.File, acc []wrongMatch) []wrongMatch {
	for _, span := range lint.Layer0(f).BlockSpans {
		switch span.Kind {
		case lint.BlockFencedCode:
			if r.CheckCode {
				acc = r.scanFencedSpan(entries, f, span, acc)
			}
		case lint.BlockIndentedCode:
			if r.CheckCode {
				acc = r.scanIndentedSpan(entries, f, span, acc)
			}
		case lint.BlockHTML:
			if r.CheckHTML {
				acc = scanRawLines(entries, f, span.Start, span.End, acc)
			}
		}
	}
	return acc
}

// scanFencedSpan scans a fenced code block's body lines (fence lines excluded),
// stripping the opening fence's leading indent from each body line so the
// content offset matches goldmark's recorded segment.
func (r *Rule) scanFencedSpan(entries []nameEntry, f *lint.File, span lint.BlockSpan, acc []wrongMatch) []wrongMatch {
	bodyEnd := span.End
	if span.Closed {
		bodyEnd = span.End - 1
	}
	indent := leadingSpaces(f.Lines[span.Start-1])
	for ln := span.Start + 1; ln <= bodyEnd; ln++ {
		line := f.Lines[ln-1]
		strip := indent
		if n := leadingSpaces(line); n < strip {
			strip = n
		}
		start := f.LineStartOffset(ln-1) + strip
		acc = append(acc, scanBytes(entries, f.Source[start:start+len(line)-strip], start, f.Source)...)
	}
	return acc
}

// scanIndentedSpan scans an indented code block, stripping the leading
// four-space / single-tab indent goldmark removes from each line.
func (r *Rule) scanIndentedSpan(entries []nameEntry, f *lint.File, span lint.BlockSpan, acc []wrongMatch) []wrongMatch {
	for ln := span.Start; ln <= span.End; ln++ {
		line := f.Lines[ln-1]
		strip := indentedStrip(line)
		start := f.LineStartOffset(ln-1) + strip
		acc = append(acc, scanBytes(entries, f.Source[start:start+len(line)-strip], start, f.Source)...)
	}
	return acc
}

// scanRawLines scans lines [from, to] (1-based inclusive) verbatim, at each
// line's start offset.
func scanRawLines(entries []nameEntry, f *lint.File, from, to int, acc []wrongMatch) []wrongMatch {
	for ln := from; ln <= to; ln++ {
		start := f.LineStartOffset(ln - 1)
		acc = append(acc, scanBytes(entries, f.Lines[ln-1], start, f.Source)...)
	}
	return acc
}

// leadingSpaces counts the leading ASCII spaces of line.
func leadingSpaces(line []byte) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

// indentedStrip returns the byte count goldmark strips from an indented code
// line: a leading tab, or up to four leading spaces.
func indentedStrip(line []byte) int {
	if len(line) > 0 && line[0] == '\t' {
		return 1
	}
	n := 0
	for n < 4 && n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

// Check implements rule.Rule. On the parse-skipped path (f.AST nil) the match
// set is gathered from the shared run-grouped inline parse and the Layer 0
// block spans (collectMatchesInline) rather than the tree, byte-identical to
// the AST walk.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var matches []wrongMatch
	if f != nil && f.AST == nil {
		matches = normalizeMatches(r.collectMatchesInline(f))
	} else {
		matches = normalizeMatches(r.collectMatches(f))
	}
	if len(matches) == 0 {
		return nil
	}
	diags := make([]lint.Diagnostic, 0, len(matches))
	for _, m := range matches {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     f.LineOfOffset(m.start),
			Column:   f.ColumnOfOffset(m.start),
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: "proper name " + strconv.Quote(string(f.Source[m.start:m.start+m.length])) +
				" should be " + strconv.Quote(m.name),
		})
	}
	return diags
}

// Fix implements rule.FixableRule. It replaces each wrong-cased match with
// the canonical spelling. Only the left boundary is enforced — the byte
// before the match must be a non-word character (or start of file), but no
// right-boundary check is applied. The matched prefix is replaced and any
// trailing word characters (e.g. the 's' in "JavaScripts") are left as-is.
func (r *Rule) Fix(f *lint.File) []byte {
	matches := normalizeMatches(r.collectMatches(f))
	if len(matches) == 0 {
		out := make([]byte, len(f.Source))
		copy(out, f.Source)
		return out
	}

	var out bytes.Buffer
	prev := 0
	for _, m := range matches {
		out.Write(f.Source[prev:m.start])
		out.WriteString(m.name)
		prev = m.start + m.length
	}
	out.Write(f.Source[prev:])
	return out.Bytes()
}

// ApplySettings implements rule.Configurable. The names list appends across
// config layers (same as placeholders:) so kind layers can extend the
// inherited vocabulary rather than replace it.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "names":
			names, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("proper-names: names must be a list of strings, got %T", v)
			}
			r.Names = names
		case "check-code":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("proper-names: check-code must be a bool, got %T", v)
			}
			r.CheckCode = b
		case "check-html":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("proper-names: check-html must be a bool, got %T", v)
			}
			r.CheckHTML = b
		default:
			return fmt.Errorf("proper-names: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"names":      []string{},
		"check-code": false,
		"check-html": false,
	}
}

// SettingMergeMode implements rule.ListMerger. The names list appends across
// config layers so kind layers extend the inherited vocabulary without
// replacing it.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "names" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

var (
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Fix proper-name capitalization" }
