package nobareurls

import (
	"bytes"
	"regexp"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

var urlPattern = regexp.MustCompile(`https?://[^\s)>\]]+`)

// urlNeedle is the literal prefix every urlPattern match carries.
var urlNeedle = []byte("http")

// mayContainURL reports whether content could contain a urlPattern
// match. Every match starts with "http", so this literal check spares
// the regex engine on the overwhelming majority of text nodes. Shared
// by flagTextNode (Check) and Fix so the two paths can't drift apart
// on which nodes get scanned.
func mayContainURL(content []byte) bool {
	return bytes.Contains(content, urlNeedle)
}

// Rule checks that bare URLs in text are flagged.
// URLs inside links, code blocks, code spans, autolinks, or reference
// definitions are not considered bare.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS012" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-bare-urls" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// Check implements rule.Rule. The per-text-node logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// On the parse-skipped path (f.AST nil) the AST walk surfaces no
	// nodes, so serve from the shared run-grouped inline parse: the same
	// Text-node logic runs over every parsed run, with run-local segment
	// offsets mapped back to the document. Re-using goldmark's own parser
	// keeps the flagged bare-URL set byte-identical to the AST path.
	if f.AST == nil {
		var diags []lint.Diagnostic
		lint.WalkInlineNodes(f, func(n ast.Node, base int) {
			diags = append(diags, r.flagTextNode(n, f, base)...)
		})
		return diags
	}
	return rule.WalkNodes(r, f)
}

// InlineCapable implements rule.InlineChecker: Check serves the nil-AST
// path from lint.WalkInlineNodes (which reads lint.InlineBlocks).
func (r *Rule) InlineCapable() bool { return true }

var _ rule.InlineChecker = (*Rule)(nil)

// CheckNode implements rule.NodeChecker. On the AST path the segment
// offsets are already document-absolute, so the base offset is zero.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	return r.flagTextNode(n, f, 0)
}

// flagTextNode emits a diagnostic for each bare URL inside a Text node that
// is not within a link, autolink, or code span. base is added to the node's
// segment-local offsets to recover document-absolute positions: zero on the
// AST path (segments are already absolute), the run's start offset on the
// per-block path. The node's content is read against f.Source on the AST
// path and against the same Source via the absolute offsets on the per-block
// path — both index the identical bytes.
func (r *Rule) flagTextNode(n ast.Node, f *lint.File, base int) []lint.Diagnostic {
	textNode, ok := n.(*ast.Text)
	if !ok {
		return nil
	}
	// Skip text nodes inside links.
	if isInsideNonBareContext(n) {
		return nil
	}

	seg := textNode.Segment
	absStart := base + seg.Start
	absStop := base + seg.Stop
	content := f.Source[absStart:absStop]
	if !mayContainURL(content) {
		return nil
	}
	matches := urlPattern.FindAllIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}

	diags := make([]lint.Diagnostic, 0, len(matches))
	for _, m := range matches {
		offset := absStart + m[0]
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     f.LineOfOffset(offset),
			Column:   f.ColumnOfOffset(offset),
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "bare URL — wrap in angle brackets or add link text",
		})
	}
	return diags
}

var _ rule.NodeChecker = (*Rule)(nil)

// isInsideNonBareContext checks if a node is a descendant of an ast.Link,
// ast.AutoLink, or ast.CodeSpan (where URLs should not be flagged).
func isInsideNonBareContext(n ast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.(type) {
		case *ast.Link, *ast.AutoLink, *ast.CodeSpan:
			return true
		}
	}
	return false
}

// FixTitle implements rule.QuickFixTitler so the editor lightbulb reads
// "Wrap in angle brackets" rather than the generic "Fix all ...".
func (r *Rule) FixTitle() string { return "Wrap in angle brackets" }

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	type replacement struct {
		start int
		end   int
		url   []byte
	}
	var replacements []replacement

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		textNode, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}

		if isInsideNonBareContext(n) {
			return ast.WalkContinue, nil
		}

		seg := textNode.Segment
		content := seg.Value(f.Source)
		if !mayContainURL(content) {
			return ast.WalkContinue, nil
		}
		matches := urlPattern.FindAllIndex(content, -1)
		for _, m := range matches {
			absStart := seg.Start + m[0]
			absEnd := seg.Start + m[1]
			replacements = append(replacements, replacement{
				start: absStart,
				end:   absEnd,
				url:   f.Source[absStart:absEnd],
			})
		}

		return ast.WalkContinue, nil
	})

	if len(replacements) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	// Build result by applying replacements in order (they are already in
	// document order from the AST walk). Each replacement adds exactly
	// two bytes (`<` and `>`) over the source it wraps, so the final
	// size is known before the first append.
	result := make([]byte, 0, len(f.Source)+2*len(replacements))
	prev := 0
	for _, rep := range replacements {
		result = append(result, f.Source[prev:rep.start]...)
		result = append(result, '<')
		result = append(result, rep.url...)
		result = append(result, '>')
		prev = rep.end
	}
	result = append(result, f.Source[prev:]...)
	return result
}

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindText}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
