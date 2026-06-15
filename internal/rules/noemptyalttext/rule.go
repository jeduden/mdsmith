package noemptyalttext

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that images have non-empty alt text for accessibility.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS032" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-empty-alt-text" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "accessibility" }

// Check implements rule.Rule. The per-image logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// On the parse-skipped path (f.AST nil) the AST walk surfaces no
	// nodes, so drive the per-block dispatch over the Layer 0 block scan:
	// each inline-bearing span is parsed in isolation and the same Image
	// logic runs over it, with span-local segment offsets mapped back to
	// the document. Re-using goldmark's parser per span keeps the flagged
	// empty-alt set byte-identical to the AST path.
	if f.AST == nil {
		return rule.WalkBlocks(r, f)
	}
	return rule.WalkNodes(r, f)
}

// CheckBlock implements rule.BlockChecker. It parses the span's own source
// bytes in isolation and applies the per-Image empty-alt check, mapping
// span-local segment offsets to the document via the span's start offset.
func (r *Rule) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	start := f.LineStartOffset(span.Start - 1)
	end := f.LineEndOffset(span.End - 1)
	if end <= start {
		return nil
	}
	doc := lint.ParseInline(f.Source[start:end])
	var diags []lint.Diagnostic
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if d, ok := r.checkImage(n, f, start); ok {
			diags = append(diags, d)
		}
		return ast.WalkContinue, nil
	})
	if len(diags) == 0 {
		return nil
	}
	return diags
}

// blockKinds is the static block-kind interest CheckBlock declares via
// rule.BlockChecker; package-level so BlockKinds returns it without
// allocating. An image can appear in body text under any of these block
// kinds.
var blockKinds = []lint.BlockKind{
	lint.BlockParagraph,
	lint.BlockATXHeading,
	lint.BlockSetextHeading,
	lint.BlockList,
	lint.BlockQuote,
}

// BlockKinds implements rule.BlockChecker.
func (r *Rule) BlockKinds() []lint.BlockKind { return blockKinds }

var _ rule.BlockChecker = (*Rule)(nil)

// CheckNode implements rule.NodeChecker. On the AST path segment offsets
// are already document-absolute, so the base offset is zero.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	if d, ok := r.checkImage(n, f, 0); ok {
		return []lint.Diagnostic{d}
	}
	return nil
}

var _ rule.NodeChecker = (*Rule)(nil)

// checkImage emits a diagnostic when n is an image with empty (or
// whitespace-only) alt text. base is added to the node's segment-local
// offsets to recover document-absolute positions: zero on the AST path,
// the span's start offset on the per-block path.
func (r *Rule) checkImage(n ast.Node, f *lint.File, base int) (lint.Diagnostic, bool) {
	img, ok := n.(*ast.Image)
	if !ok {
		return lint.Diagnostic{}, false
	}
	if strings.TrimSpace(imageAltText(img, f, base)) != "" {
		return lint.Diagnostic{}, false
	}
	return lint.Diagnostic{
		File:     f.Path,
		Line:     imageLine(img, f, base),
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "image has empty alt text",
	}, true
}

func imageAltText(img *ast.Image, f *lint.File, base int) string {
	var b strings.Builder
	collectText(&b, img, f.Source, base)
	return b.String()
}

func collectText(b *strings.Builder, n ast.Node, source []byte, base int) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(source[base+t.Segment.Start : base+t.Segment.Stop])
		} else {
			collectText(b, c, source, base)
		}
	}
}

// isInlineNode returns true for inline AST nodes whose Lines() panics.
func isInlineNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.Text, *ast.String, *ast.CodeSpan, *ast.Emphasis,
		*ast.Link, *ast.Image, *ast.AutoLink, *ast.RawHTML:
		return true
	}
	return false
}

func imageLine(img *ast.Image, f *lint.File, base int) int {
	// Try child text nodes first for precise position.
	line := firstTextLine(img, f, base)
	if line > 0 {
		return line
	}
	// Walk up ancestors, skipping inline nodes whose Lines() panics.
	for p := img.Parent(); p != nil; p = p.Parent() {
		if isInlineNode(p) {
			continue
		}
		lines := p.Lines()
		if lines != nil && lines.Len() > 0 {
			return f.LineOfOffset(base + lines.At(0).Start)
		}
	}
	return 1
}

func firstTextLine(n ast.Node, f *lint.File, base int) int {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(base + t.Segment.Start)
		}
		if line := firstTextLine(c, f, base); line > 0 {
			return line
		}
	}
	return 0
}

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindImage}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
