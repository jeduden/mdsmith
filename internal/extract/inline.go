package extract

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/yuin/goldmark/ast"
)

// inlineSpans walks the inline children of a paragraph node and
// returns them as a typed, recursive span list. Container spans
// (emphasis, strong, link) carry a `children` list; leaf spans
// (text, code, autolink) carry a `value`. An unsupported inline node
// (image, raw HTML, or any node outside the documented mapping) is a
// hard error: walkInlineChildren records a projection diagnostic via
// the projector and the caller returns nothing. Plan 212.
//
// The walker drives `mdsmith extract` only — it never runs on the
// check hot path — so it favours a clear recursive shape over the
// allocation budget the lint rules hold to.
func (p *projector) inlineSpans(n ast.Node) []any {
	return p.walkInlineChildren(n)
}

// walkInlineChildren maps every inline child of parent to a span
// object and returns the ordered list. A child outside the mapping
// table records a collision-style diagnostic and is skipped; because
// Extract treats any diagnostic as a hard failure and emits nothing,
// the partial list is never surfaced.
//
// A single Text node can contribute two spans: its text span, then a
// `break` span when the node carries a soft or hard line break. The
// break span is appended after the text span so the wrapped-line
// structure of a paragraph survives projection.
func (p *projector) walkInlineChildren(parent ast.Node) []any {
	var spans []any
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		span := p.inlineSpan(c)
		if span == nil {
			continue
		}
		spans = append(spans, span)
		if br := breakSpan(c); br != nil {
			spans = append(spans, br)
		}
	}
	return spans
}

// breakSpan returns a `break` span when n is a Text node ending in a
// soft or hard line break, and nil otherwise. `hard` is true for a
// hard break (a backslash or two trailing spaces before the newline)
// and false for a soft break (a plain wrapped line). Plan 212.
func breakSpan(n ast.Node) map[string]any {
	t, ok := n.(*ast.Text)
	if !ok {
		return nil
	}
	switch {
	case t.HardLineBreak():
		return map[string]any{"span": "break", "hard": true}
	case t.SoftLineBreak():
		return map[string]any{"span": "break", "hard": false}
	default:
		return nil
	}
}

// inlineSpan maps one inline AST node to its span object per the plan
// 212 mapping table. It returns nil (after recording a diagnostic)
// for any node the table does not cover.
func (p *projector) inlineSpan(n ast.Node) map[string]any {
	switch node := n.(type) {
	case *ast.Text:
		return map[string]any{
			"span":  "text",
			"value": string(node.Segment.Value(p.f.Source)),
		}
	case *ast.String:
		// String nodes carry their payload inline (typographer /
		// autolink transformers emit them); treat them as text so a
		// rewritten span still projects.
		return map[string]any{"span": "text", "value": string(node.Value)}
	case *ast.CodeSpan:
		return map[string]any{"span": "code", "value": p.codeSpanText(node)}
	case *ast.AutoLink:
		url := string(node.URL(p.f.Source))
		return map[string]any{
			"span":  "autolink",
			"value": string(node.Label(p.f.Source)),
			"url":   url,
		}
	case *ast.Emphasis:
		name := "emphasis"
		if node.Level == 2 {
			name = "strong"
		}
		return map[string]any{
			"span":     name,
			"level":    node.Level,
			"children": p.walkInlineChildren(node),
		}
	case *ast.Link:
		span := map[string]any{
			"span":     "link",
			"url":      string(node.Destination),
			"children": p.walkInlineChildren(node),
		}
		if len(node.Title) > 0 {
			span["title"] = string(node.Title)
		}
		return span
	default:
		p.unsupportedInline(n)
		return nil
	}
}

// codeSpanText concatenates a code span's text segments verbatim.
func (p *projector) codeSpanText(n *ast.CodeSpan) string {
	var b []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b = append(b, t.Segment.Value(p.f.Source)...)
		}
	}
	return string(b)
}

// unsupportedInline records a hard projection error naming the node
// type the inline projection cannot represent. Images and inline raw
// HTML are the common cases; the default branch names the Go type so
// a future custom inline node surfaces a clear message.
func (p *projector) unsupportedInline(n ast.Node) {
	var what string
	switch n.(type) {
	case *ast.Image:
		what = "an image"
	case *ast.RawHTML:
		what = "inline raw HTML"
	default:
		what = fmt.Sprintf("an unsupported inline node (%T)", n)
	}
	p.emit(schema.SchemaDiagnostic{
		Field:    "inline",
		Actual:   what,
		Expected: "one of: text, code, autolink, emphasis, strong, link",
		Hint: "the `projection: inline` mapping covers only those " +
			"spans; remove the node or drop the inline projection",
		SchemaRef: schema.FormatSchemaRef(p.sch, ""),
	})
}
