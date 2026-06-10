package extract

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// blocksFromNodes maps a document-ordered slice of block-level AST
// nodes to the typed `blocks` list (plan 246's block grammar). It is
// the block-level analogue of inlineSpans: container blocks
// (blockquote, deeper-heading section) recurse through the same
// grammar, leaves carry their own payload.
//
// A heading node in the slice opens a nested `section` block: every
// following node, up to the next heading of the same or a shallower
// level, becomes that section's recursive `blocks`. So a heading and
// the body beneath it nest under one `section` object rather than
// flattening into siblings. The caller bounds the slice to the
// enclosing scope's body, so every heading here is deeper than that
// scope.
//
// The walker drives `mdsmith extract` only — never the check hot
// path — so it favours a clear recursive shape over the allocation
// budget the lint rules hold to.
func (p *projector) blocksFromNodes(nodes []ast.Node, inline bool) []any {
	blocks := make([]any, 0, len(nodes))
	for i := 0; i < len(nodes); {
		n := nodes[i]
		if h, ok := n.(*ast.Heading); ok {
			// Gather the heading's body: siblings up to the next
			// heading of the same or shallower level.
			j := i + 1
			for j < len(nodes) && !headingAtOrAbove(nodes[j], h.Level) {
				j++
			}
			blocks = append(blocks, p.sectionBlock(h, nodes[i+1:j], inline))
			i = j
			continue
		}
		if b := p.blockNode(n, inline); b != nil {
			blocks = append(blocks, b)
		}
		i++
	}
	return blocks
}

// headingAtOrAbove reports whether n is a heading whose level is <=
// level — the boundary that closes a nested `section` block's body.
func headingAtOrAbove(n ast.Node, level int) bool {
	h, ok := n.(*ast.Heading)
	return ok && h.Level <= level
}

// blocksFromChildren maps every block child of parent. Container
// blocks (blockquote) recurse through it. A blockquote body has no
// headings in practice, but routing through blocksFromNodes keeps the
// section-nesting behaviour uniform if one appears.
func (p *projector) blocksFromChildren(parent ast.Node, inline bool) []any {
	var nodes []ast.Node
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		nodes = append(nodes, c)
	}
	return p.blocksFromNodes(nodes, inline)
}

// blockNode maps one block-level AST node to its block object per the
// plan 246 grammar table. It returns nil (after recording a
// diagnostic) for any node the table does not cover, mirroring
// inlineSpan's hard-error contract. inline selects the paragraph
// rendering (`block-paragraphs: inline`) and flows through container
// recursion.
func (p *projector) blockNode(n ast.Node, inline bool) map[string]any {
	switch node := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return p.paragraphBlock(n, inline)
	case *ast.FencedCodeBlock:
		return p.fencedCodeBlock(node)
	case *ast.CodeBlock:
		return p.indentedCodeBlock(node)
	case *ast.List:
		return map[string]any{"block": "list", "items": p.listTree(node)}
	case *extast.Table:
		cols, rows := p.tableRowsPositional(node)
		return map[string]any{"block": "table", "columns": cols, "rows": rows}
	case *ast.Blockquote:
		return map[string]any{"block": "quote", "blocks": p.blocksFromChildren(node, inline)}
	case *ast.ThematicBreak:
		return map[string]any{"block": "break"}
	case *ast.HTMLBlock:
		return map[string]any{"block": "html", "value": p.htmlBlockValue(node)}
	default:
		p.unsupportedBlock(n)
		return nil
	}
}

// paragraphBlock projects a paragraph (or a loose list item's
// TextBlock). The default is flat `text`, matching the `text` content
// projection. When the scope's `block-paragraphs` is `inline`, it
// projects the paragraph's typed inline-span list under `inline`
// instead, so block mode does not force plain text. Block-mode inline
// is lenient (lenientInlineSpans): an image projects an `image` span
// rather than aborting, so representable content never exits
// non-zero. Plan 246.
func (p *projector) paragraphBlock(n ast.Node, inline bool) map[string]any {
	if inline {
		return map[string]any{
			"block":  "paragraph",
			"inline": p.lenientInlineSpans(n),
		}
	}
	return map[string]any{"block": "paragraph", "text": p.nodeText(n)}
}

// fencedCodeBlock projects a fenced code block as `{block: code,
// lang?, value}`. The fence info string becomes `lang` when present.
// `value` keeps the body's trailing newline (the `code` content
// projection's codeBody trims it; the block grammar promises the
// verbatim body, so rawLines does not trim).
func (p *projector) fencedCodeBlock(n *ast.FencedCodeBlock) map[string]any {
	b := map[string]any{"block": "code", "value": p.rawLines(n)}
	if lang := string(n.Language(p.f.Source)); lang != "" {
		b["lang"] = lang
	}
	return b
}

// indentedCodeBlock projects an indented (non-fenced) code block. It
// has no info string, so `lang` is always absent.
func (p *projector) indentedCodeBlock(n *ast.CodeBlock) map[string]any {
	return map[string]any{"block": "code", "value": p.rawLines(n)}
}

// rawLines concatenates a block's Lines() segments verbatim.
func (p *projector) rawLines(n ast.Node) string {
	var b strings.Builder
	segs := n.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		b.Write(seg.Value(p.f.Source))
	}
	return b.String()
}

// htmlBlockValue concatenates an HTML block's raw lines plus its
// closing line (when present), so the projected `value` is the
// verbatim HTML the source carried.
func (p *projector) htmlBlockValue(n *ast.HTMLBlock) string {
	v := p.rawLines(n)
	if n.HasClosure() {
		v += string(n.ClosureLine.Value(p.f.Source))
	}
	return strings.TrimRight(v, "\n")
}

// sectionBlock projects a heading deeper than the declared schema as
// `{block: section, level, heading, blocks}`. body is the heading's
// run of following nodes (up to the next same-or-shallower heading),
// which recurse through the same grammar — so a deeper heading nested
// inside body opens its own `section` block.
func (p *projector) sectionBlock(h *ast.Heading, body []ast.Node, inline bool) map[string]any {
	return map[string]any{
		"block":   "section",
		"level":   h.Level,
		"heading": p.nodeText(h),
		"blocks":  p.blocksFromNodes(body, inline),
	}
}

// unsupportedBlock records a hard projection error naming the block
// node type the grammar cannot represent. Mirrors unsupportedInline:
// extract treats any diagnostic as a hard failure and emits nothing.
// The grammar covers every standard CommonMark + GFM-table block, so
// this fires only for a custom block node a future extension adds.
func (p *projector) unsupportedBlock(n ast.Node) {
	p.emit(schema.SchemaDiagnostic{
		Field:    "blocks",
		Actual:   fmt.Sprintf("an unsupported block node (%T)", n),
		Expected: "one of: paragraph, code, list, table, quote, break, html, section",
		Hint: "the `projection: blocks` grammar covers only those blocks; " +
			"remove the node or project the section a different way",
		SchemaRef: schema.FormatSchemaRef(p.sch, ""),
	})
}
