// Package samefileanchor implements MDS070, which checks that same-file
// #fragment links resolve to a heading in the same file. It is parse-skip-safe:
// both the AST path and the nil-AST path use only f.Lines, Layer0, and the
// shared inline-block projection — no goldmark AST walk is required.
package samefileanchor

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that every same-file #fragment link in a file resolves
// to a heading present in that file.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS070" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "same-file-anchor" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return true }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil {
		return nil
	}

	// Quick pre-filter: if the source contains no '#' byte there are no
	// same-file fragment links to check and no fragment-link anchors to
	// worry about. The early exit avoids building the slug set entirely.
	if !bytes.ContainsRune(f.Source, '#') {
		return nil
	}

	// Collect all heading slugs from the file.
	slugs := collectSlugs(f)

	if f.AST == nil {
		return r.checkNilAST(f, slugs)
	}
	return r.checkAST(f, slugs)
}

// checkAST walks the goldmark AST to find same-file #fragment links.
func (r *Rule) checkAST(f *lint.File, slugs map[string]struct{}) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest []byte
		switch v := n.(type) {
		case *ast.Link:
			dest = v.Destination
		case *ast.Image:
			dest = v.Destination
		default:
			return ast.WalkContinue, nil
		}
		fragment := sameFileFragment(dest)
		if fragment == nil || len(fragment) == 0 {
			// nil: not a same-file fragment; empty: bare # (top-of-page), always valid
			return ast.WalkContinue, nil
		}
		if _, found := slugs[string(fragment)]; !found {
			line := inlineNodeLine(n, f, 0)
			diags = append(diags, r.diag(f, line, string(fragment)))
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// checkNilAST handles the parse-skip path by using the shared InlineBlocks projection.
func (r *Rule) checkNilAST(f *lint.File, slugs map[string]struct{}) []lint.Diagnostic {
	var diags []lint.Diagnostic
	lint.WalkInlineNodes(f, func(n ast.Node, base int) {
		var dest []byte
		switch v := n.(type) {
		case *ast.Link:
			dest = v.Destination
		case *ast.Image:
			dest = v.Destination
		default:
			return
		}
		fragment := sameFileFragment(dest)
		if fragment == nil || len(fragment) == 0 {
			return
		}
		if _, found := slugs[string(fragment)]; !found {
			line := inlineNodeLine(n, f, base)
			diags = append(diags, r.diag(f, line, string(fragment)))
		}
	})
	return diags
}

// sameFileFragment returns the fragment portion of dest (without leading '#')
// when dest is a same-file fragment reference (starts with '#' and has no
// path component). Returns nil when dest is not a same-file fragment.
func sameFileFragment(dest []byte) []byte {
	if len(dest) == 0 || dest[0] != '#' {
		return nil
	}
	return dest[1:]
}

// slugBuf is a package-level stack buffer used by appendSlug to avoid a
// heap allocation for the temporary slug bytes before they are interned into
// the map. Since rules run serially per File (one goroutine per Check call),
// this is safe without synchronisation. The buffer is 512 bytes — long enough
// for any realistic heading line (the line-length rule caps lines at 80–120
// chars for the default settings).
var slugBuf [512]byte

// collectSlugs builds the set of heading slugs that exist in the file.
// On the AST path it walks the goldmark tree; on the nil-AST path it uses
// the Layer 0 block spans. Both paths produce the same slug set.
func collectSlugs(f *lint.File) map[string]struct{} {
	if f.AST != nil {
		return collectSlugsAST(f)
	}
	return collectSlugsLayer0(f)
}

// collectSlugsAST walks the goldmark AST to collect heading slugs.
// The walk uses direct recursion (not ast.Walk) to avoid the closure alloc.
func collectSlugsAST(f *lint.File) map[string]struct{} {
	slugs := make(map[string]struct{}, 4)
	collectSlugsNode(f.AST, f.Source, &slugs)
	if len(slugs) == 0 {
		return nil
	}
	return slugs
}

// collectSlugsNode descends into n and fills the slugs map for every heading.
// Direct recursion (instead of ast.Walk) avoids the per-node closure allocation.
func collectSlugsNode(n ast.Node, src []byte, slugs *map[string]struct{}) {
	if n == nil {
		return
	}
	if h, ok := n.(*ast.Heading); ok {
		text := headingTextFromAST(h, src)
		slug := appendSlug(slugBuf[:0], text)
		if len(slug) > 0 {
			(*slugs)[string(slug)] = struct{}{}
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectSlugsNode(c, src, slugs)
	}
}

// headingTextFromAST collects the plain-text content of a heading node by
// concatenating its Text child segments. It writes into slugBuf2 to avoid
// an allocation when the heading text is short.
var slugBuf2 [256]byte

func headingTextFromAST(h *ast.Heading, src []byte) []byte {
	// Collect text segments into slugBuf2 (reset each call).
	dst := slugBuf2[:0]
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			dst = append(dst, src[t.Segment.Start:t.Segment.Stop]...)
		}
	}
	return dst
}

// collectSlugsLayer0 uses the Layer 0 block spans to collect heading slugs.
func collectSlugsLayer0(f *lint.File) map[string]struct{} {
	l0 := lint.Layer0(f)
	// Count headings first so the map is pre-sized exactly.
	headingCount := 0
	for i := range l0.BlockSpans {
		k := l0.BlockSpans[i].Kind
		if k == lint.BlockATXHeading || k == lint.BlockSetextHeading {
			headingCount++
		}
	}
	if headingCount == 0 {
		return nil
	}
	slugs := make(map[string]struct{}, headingCount)
	for _, span := range l0.BlockSpans {
		switch span.Kind {
		case lint.BlockATXHeading:
			line := f.Lines[span.Start-1]
			text := atxHeadingText(line)
			slug := appendSlug(slugBuf[:0], text)
			if len(slug) > 0 {
				slugs[string(slug)] = struct{}{}
			}
		case lint.BlockSetextHeading:
			// The setext heading text is on the first line of the span;
			// the underline is the last line.
			if span.Start <= span.End-1 {
				line := f.Lines[span.Start-1]
				slug := appendSlug(slugBuf[:0], bytes.TrimSpace(line))
				if len(slug) > 0 {
					slugs[string(slug)] = struct{}{}
				}
			}
		}
	}
	return slugs
}

// atxHeadingText extracts the visible text from an ATX heading line.
// It strips the leading '#' markers and optional space, and trims any
// trailing closing '#' markers as well as leading/trailing whitespace.
// It returns a subslice of line (no allocation).
func atxHeadingText(line []byte) []byte {
	// Strip leading spaces (up to 3).
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	// Skip '#' markers.
	for i < len(line) && line[i] == '#' {
		i++
	}
	// Skip one optional space/tab after the markers.
	if i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	text := line[i:]
	// Trim trailing carriage return / newline.
	text = bytes.TrimRight(text, "\r\n")
	// Trim trailing closing '#' sequence (e.g. "## Heading ##").
	j := len(text)
	for j > 0 && text[j-1] == '#' {
		j--
	}
	if j < len(text) {
		// There was a trailing '#' run — strip any preceding whitespace too.
		k := j
		for k > 0 && (text[k-1] == ' ' || text[k-1] == '\t') {
			k--
		}
		text = text[:k]
	}
	return bytes.TrimSpace(text)
}

// appendSlug appends the GitHub-flavored Markdown anchor slug for text onto dst
// and returns the extended slice. Slug rules: lowercase, spaces → '-',
// keep ASCII letters, digits, '-'; keep non-ASCII letters and numbers;
// drop everything else. No allocation when dst has enough capacity.
func appendSlug(dst, text []byte) []byte {
	for i := 0; i < len(text); {
		b := text[i]
		if b < utf8.RuneSelf {
			// Single-byte rune (ASCII).
			switch {
			case b == ' ':
				dst = append(dst, '-')
			case b == '-' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9'):
				dst = append(dst, b)
			case b >= 'A' && b <= 'Z':
				dst = append(dst, b+'a'-'A') // lowercase
			// all other ASCII bytes (punctuation, etc.) are dropped
			}
			i++
			continue
		}
		// Multi-byte rune.
		r, size := utf8.DecodeRune(text[i:])
		if r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsNumber(r)) {
			// Lowercase non-ASCII letters.
			lo := unicode.ToLower(r)
			var buf [utf8.UTFMax]byte
			n := utf8.EncodeRune(buf[:], lo)
			dst = append(dst, buf[:n]...)
		}
		i += size
	}
	return dst
}

// inlineNodeLine returns the 1-based source line of an inline node by
// scanning its children for a Text node with a segment offset. base is
// added to segment offsets to recover document-absolute positions on the
// nil-AST (inline-block) path; it is zero on the AST path. Falls back
// to line 1 when no text child can be found.
func inlineNodeLine(n ast.Node, f *lint.File, base int) int {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(base + t.Segment.Start)
		}
		if ln := inlineNodeLine(c, f, base); ln > 1 {
			return ln
		}
	}
	return 1
}

// diag builds a diagnostic for an unresolved fragment.
func (r *Rule) diag(f *lint.File, line int, fragment string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "same-file anchor #" + fragment + " does not match any heading in this file",
	}
}

var _ rule.Defaultable = (*Rule)(nil)
