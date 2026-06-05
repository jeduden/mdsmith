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
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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
// HTMLBlock) and appends any wrong-cased matches to out.
func scanLines(entries []nameEntry, n lineNode, f *lint.File) []wrongMatch {
	segs := n.Lines()
	var out []wrongMatch
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		out = append(out, scanBytes(entries, seg.Value(f.Source), seg.Start, f.Source)...)
	}
	return out
}

// scanCodeSpanChildren scans the Text children of a CodeSpan node.
func scanCodeSpanChildren(entries []nameEntry, v *ast.CodeSpan, f *lint.File) []wrongMatch {
	var out []wrongMatch
	for c := v.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		seg := t.Segment
		out = append(out, scanBytes(entries, seg.Value(f.Source), seg.Start, f.Source)...)
	}
	return out
}

// scanRawHTMLSegments scans the Segments of a RawHTML node.
func scanRawHTMLSegments(entries []nameEntry, v *ast.RawHTML, f *lint.File) []wrongMatch {
	var out []wrongMatch
	for i := 0; i < v.Segments.Len(); i++ {
		seg := v.Segments.At(i)
		out = append(out, scanBytes(entries, seg.Value(f.Source), seg.Start, f.Source)...)
	}
	return out
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
				all = append(all, scanCodeSpanChildren(entries, v, f)...)
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			if r.CheckCode {
				all = append(all, scanLines(entries, n, f)...)
			}
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock:
			if r.CheckHTML {
				all = append(all, scanLines(entries, n, f)...)
			}
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			if r.CheckHTML {
				all = append(all, scanRawHTMLSegments(entries, v, f)...)
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

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	matches := normalizeMatches(r.collectMatches(f))
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
