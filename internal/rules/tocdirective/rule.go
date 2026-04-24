// Package tocdirective implements MDS035, which flags renderer-specific
// table-of-contents directives that render as literal text on CommonMark
// and goldmark.
package tocdirective

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func init() {
	rule.Register(&Rule{})
}

// tocVariant pairs a line-level detection regex with the exact directive
// token echoed back in diagnostics.
type tocVariant struct {
	pattern *regexp.Regexp
	token   string
	// isLinkRefCandidate marks `[TOC]`, which is syntactically a valid
	// CommonMark shortcut reference link and must be suppressed when a
	// matching link reference definition exists.
	isLinkRefCandidate bool
}

// variants lists the four renderer-specific TOC directives detected by the
// rule. The regex anchors ensure each directive occupies the entire line
// (trailing whitespace allowed); anything else on the line rules it out.
var variants = []tocVariant{
	{pattern: regexp.MustCompile(`^\[TOC\][ \t]*$`), token: "[TOC]", isLinkRefCandidate: true},
	{pattern: regexp.MustCompile(`^\[\[_TOC_\]\][ \t]*$`), token: "[[_TOC_]]"},
	{pattern: regexp.MustCompile(`^\[\[toc\]\][ \t]*$`), token: "[[toc]]"},
	{pattern: regexp.MustCompile(`^\$\{toc\}[ \t]*$`), token: "${toc}"},
}

// Rule detects renderer-specific TOC directives.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS035" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "toc-directive" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}

	hasTOCRef := hasTOCLinkReference(f.Source)

	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		lines := para.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			lineText := strings.TrimRight(
				string(seg.Value(f.Source)), "\r\n",
			)
			v, matched := matchVariant(lineText)
			if !matched {
				continue
			}
			if v.isLinkRefCandidate && hasTOCRef {
				continue
			}
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     f.LineOfOffset(seg.Start),
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  buildMessage(v.token),
			})
		}
		return ast.WalkContinue, nil
	})
	return diags
}

func matchVariant(line string) (tocVariant, bool) {
	for _, v := range variants {
		if v.pattern.MatchString(line) {
			return v, true
		}
	}
	return tocVariant{}, false
}

func buildMessage(token string) string {
	return fmt.Sprintf(
		"unsupported TOC directive `%s`; use `<?toc?>` (MDS038)",
		token,
	)
}

// Fix implements rule.FixableRule. Each matched TOC directive token on its
// own line is replaced with an empty <?toc?>\n<?/toc?> block. Blank lines
// are inserted above and below when adjacent content would otherwise fuse
// the block into a paragraph. Only replaces tokens inside Paragraph nodes
// (same as Check), avoiding code blocks and other contexts.
func (r *Rule) Fix(f *lint.File) []byte {
	if f == nil || f.AST == nil {
		return nil
	}
	hasTOCRef := hasTOCLinkReference(f.Source)

	// Collect byte offsets of all TOC directive lines that need replacement.
	replacements := collectReplacements(f, hasTOCRef)

	if len(replacements) == 0 {
		return f.Source
	}

	return buildFixedSource(f.Source, replacements)
}

// collectReplacements scans the AST for TOC directive tokens that need replacement.
func collectReplacements(f *lint.File, hasTOCRef bool) []struct{ start, end int } {
	var replacements []struct {
		start, end int
	}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		lines := para.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			lineText := strings.TrimRight(
				string(seg.Value(f.Source)), "\r\n",
			)
			v, matched := matchVariant(lineText)
			if !matched {
				continue
			}
			if v.isLinkRefCandidate && hasTOCRef {
				continue
			}
			replacements = append(replacements, struct{ start, end int }{
				start: seg.Start,
				end:   seg.Stop,
			})
		}
		return ast.WalkContinue, nil
	})
	return replacements
}

// buildFixedSource constructs the fixed source by replacing all collected segments.
func buildFixedSource(source []byte, replacements []struct{ start, end int }) []byte {
	// Build result by copying source and replacing matched segments.
	var result bytes.Buffer
	pos := 0
	for _, repl := range replacements {
		// Copy everything before this replacement.
		result.Write(source[pos:repl.start])

		// Determine if we need blank lines around the replacement.
		needBlankBefore, needBlankAfter := computeBlankLines(source, repl.start, repl.end)

		// Write replacement with optional blank lines.
		if needBlankBefore {
			result.WriteString("\n")
		}
		result.WriteString("<?toc?>\n<?/toc?>\n")
		if needBlankAfter {
			result.WriteString("\n")
		}

		// Skip past the replaced segment. The segment itself doesn't include
		// the line's trailing newline, but we've already written a newline in
		// our replacement, so we need to skip one newline after the segment.
		pos = repl.end
		if pos < len(source) && source[pos] == '\n' {
			pos++
		}
	}
	// Copy remainder.
	result.Write(source[pos:])

	return result.Bytes()
}

// computeBlankLines determines if blank lines are needed before and after
// a replacement segment to avoid fusing it into surrounding paragraphs.
func computeBlankLines(source []byte, start, end int) (needBefore, needAfter bool) {
	// Check if there's non-blank content before this segment.
	// Only add a blank line if the immediately preceding line has content.
	if start > 0 {
		// Count trailing newlines.
		trailingNewlines := 0
		for i := start - 1; i >= 0 && source[i] == '\n'; i-- {
			trailingNewlines++
		}
		// If there are 0 newlines, or 1 newline (meaning line directly above has content),
		// we need a blank line. If there are 2+ newlines, there's already spacing.
		needBefore = (trailingNewlines < 2)
	}

	// Check if there's non-blank content after this segment.
	if end < len(source) {
		// If next char is newline, there's already a blank line.
		// If next char is not newline, next line has content - need blank.
		if source[end] != '\n' {
			needAfter = true
		}
	}

	return needBefore, needAfter
}

var _ rule.FixableRule = (*Rule)(nil)

// hasTOCLinkReference returns true when the document defines a link
// reference with label "TOC" (CommonMark-normalized). It re-parses with
// lint.NewParser so the parser configuration (including mdsmith's PI
// block parser) matches the original lint parse; otherwise content
// absorbed into a processing-instruction block could register as a link
// reference here while being hidden from the rule's AST walk.
func hasTOCLinkReference(source []byte) bool {
	if len(source) == 0 {
		return false
	}
	ctx := parser.NewContext()
	lint.NewParser().Parse(text.NewReader(source), parser.WithContext(ctx))
	_, ok := ctx.Reference("toc")
	return ok
}

var _ rule.Defaultable = (*Rule)(nil)
