// Package extract projects a schema-conformant Markdown document
// into a data tree whose shape mirrors the composed schema
// hierarchy. It runs after a successful schema match (extraction is
// gated on conformance) and never re-matches: it consumes the
// schema.MatchTree produced by schema.BuildMatchTree.
//
// The default binding layer is intentionally annotation-free — see
// plan/166_schema-driven-data-extraction.md. Every emitted key
// flows through keyFor, the single seam a future custom-binding
// plan (plan 167) overrides.
package extract

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// Extract projects f against the composed schema sch using the
// pre-built match tree m. It returns the root data tree (a
// map[string]any) and any schema diagnostics raised during
// projection (sibling key collisions). On a collision the data
// tree is returned as-is up to the conflict; callers should treat a
// non-empty diagnostic slice as a hard failure and emit nothing.
func Extract(
	f *lint.File, sch *schema.Schema, m *schema.MatchTree,
) (any, []lint.Diagnostic) {
	p := &projector{f: f, sch: sch}
	root := map[string]any{}
	// The root always carries a `frontmatter` object beside the
	// projected sections (an empty object when the document has no
	// front matter) so the emitted shape is stable across
	// otherwise-equivalent files, per the documented contract.
	if fm := m.Frontmatter; len(fm) > 0 {
		root["frontmatter"] = fm
	} else {
		root["frontmatter"] = map[string]any{}
	}
	p.projectChildren(m.Root.Children, root)
	if len(p.diags) > 0 {
		return nil, p.diags
	}
	return root, nil
}

type projector struct {
	f     *lint.File
	sch   *schema.Schema
	diags []lint.Diagnostic
}

// keyFor is the single key-naming seam — the one function a future
// custom-binding plan (plan 167) overrides. The default binding
// derives every key from the heading: a literal heading slugifies
// whole; a placeholder-bearing heading slugifies its literal stem,
// falling back to the first `fmvar` field name when the heading is
// only a placeholder (`## {id}`).
func keyFor(sc *schema.Scope) string {
	stem, fmvars, _ := schema.HeadingStem(sc)
	if s := mdtext.Slugify(stem); s != "" {
		return s
	}
	if len(fmvars) > 0 {
		return fmvars[0]
	}
	return mdtext.Slugify(sc.Heading)
}

// isRepeating reports whether a scope projects as an array. A
// declared `repeat:` cardinality is the signal; an unset matcher
// (exactly one) projects as a single object.
func isRepeating(sc *schema.Scope) bool {
	return sc != nil && sc.Matcher != nil && sc.Matcher.Repeat.Set
}

// projectChildren walks a contiguous list of sibling scope matches,
// grouping consecutive occurrences of the same schema scope, and
// writes each group's projection into obj. A preamble group hoists
// its content directly into obj (no wrapper key).
func (p *projector) projectChildren(
	children []*schema.ScopeMatch, obj map[string]any,
) {
	i := 0
	for i < len(children) {
		sm := children[i]
		if sm.Preamble {
			p.projectContent(sm.Content, obj)
			i++
			continue
		}
		j := i + 1
		for j < len(children) && children[j].Scope == sm.Scope {
			j++
		}
		group := children[i:j]
		i = j

		key := keyFor(sm.Scope)
		if isRepeating(sm.Scope) {
			arr := make([]any, 0, len(group))
			for _, g := range group {
				arr = append(arr, p.projectScopeObject(g))
			}
			p.setKey(obj, key, arr)
			continue
		}
		if len(group) > 1 {
			p.collision(key, "duplicate heading for a non-repeating section")
			continue
		}
		p.setKey(obj, key, p.projectScopeObject(group[0]))
	}
}

// projectScopeObject builds the object value for one matched scope:
// its captured placeholders (name: value), then its child scopes,
// then its content entries.
func (p *projector) projectScopeObject(sm *schema.ScopeMatch) map[string]any {
	obj := map[string]any{}
	for name, val := range sm.Captures {
		p.setKey(obj, name, val)
	}
	p.projectChildren(sm.Children, obj)
	p.projectContent(sm.Content, obj)
	return obj
}

// projectContent projects code-block, list, table, and paragraph
// entries under their default keys. Repeated kinds get a -N suffix
// (code, code-2, …) so a second block never silently overwrites
// the first.
func (p *projector) projectContent(
	content []schema.ContentMatch, obj map[string]any,
) {
	counts := map[string]int{}
	nextKey := func(base string) string {
		counts[base]++
		if counts[base] == 1 {
			return base
		}
		return fmt.Sprintf("%s-%d", base, counts[base])
	}
	for _, cm := range content {
		switch cm.Entry.Kind {
		case schema.ContentKindCodeBlock:
			p.setKey(obj, nextKey("code"), p.codeBody(cm.Node))
		case schema.ContentKindList:
			p.setKey(obj, nextKey("items"), p.listItems(cm.Node))
		case schema.ContentKindTable:
			p.setKey(obj, nextKey("rows"), p.tableRows(cm.Node))
		case schema.ContentKindParagraph:
			p.setKey(obj, nextKey("text"), p.nodeText(cm.Node))
		}
	}
}

func (p *projector) codeBody(n ast.Node) string {
	fcb, ok := n.(*ast.FencedCodeBlock)
	if !ok {
		return ""
	}
	var b strings.Builder
	segs := fcb.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		b.Write(seg.Value(p.f.Source))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *projector) listItems(n ast.Node) []any {
	lst, ok := n.(*ast.List)
	if !ok {
		return nil
	}
	var items []any
	for c := lst.FirstChild(); c != nil; c = c.NextSibling() {
		items = append(items, strings.TrimSpace(
			mdtext.ExtractPlainText(c, p.f.Source)))
	}
	return items
}

func (p *projector) tableRows(n ast.Node) []any {
	tbl, ok := n.(*extast.Table)
	if !ok {
		return nil
	}
	var cols []string
	var rows []any
	for r := tbl.FirstChild(); r != nil; r = r.NextSibling() {
		var cells []string
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			cells = append(cells, strings.TrimSpace(
				mdtext.ExtractPlainText(c, p.f.Source)))
		}
		if _, isHeader := r.(*extast.TableHeader); isHeader {
			cols = cells
			// Duplicate column headers would collide as row-object
			// keys, silently dropping every cell but the last.
			// Surface it as a projection collision (once) rather
			// than emitting lossy rows.
			seen := make(map[string]bool, len(cols))
			for _, c := range cols {
				if c == "" {
					continue
				}
				if seen[c] {
					p.collision(c, "duplicate table column header")
				}
				seen[c] = true
			}
			continue
		}
		row := map[string]any{}
		for k, cell := range cells {
			// A GFM parser trims every body row to the header's
			// column count, so len(cells) <= len(cols) always and
			// cols[k] is in range here.
			name := fmt.Sprintf("col-%d", k+1)
			if cols[k] != "" {
				name = cols[k]
			}
			row[name] = cell
		}
		rows = append(rows, row)
	}
	return rows
}

func (p *projector) nodeText(n ast.Node) string {
	return strings.TrimSpace(mdtext.ExtractPlainText(n, p.f.Source))
}

// setKey writes val into obj under key, recording a sibling-key
// collision diagnostic instead of overwriting an existing key.
func (p *projector) setKey(obj map[string]any, key string, val any) {
	if key == "" {
		p.collision("<empty>", "scope produced an empty projection key")
		return
	}
	if _, exists := obj[key]; exists {
		p.collision(key, "two sibling projections resolve to the same key")
		return
	}
	obj[key] = val
}

func (p *projector) collision(key, why string) {
	d := schema.SchemaDiagnostic{
		Field:     key,
		Actual:    "<collision>",
		Expected:  "a unique projection key",
		Hint:      why,
		SchemaRef: schema.FormatSchemaRef(p.sch, ""),
	}
	p.diags = append(p.diags, lint.Diagnostic{
		File: p.f.Path,
		// Extract returns its diagnostics straight to the CLI
		// formatter without running them through
		// lint.File.AdjustDiagnostics, so this must already be an
		// absolute file line. schema.NonBodyDiagLine returns
		// 1-LineOffset (meant for later adjustment) and would
		// print a zero/negative line for front-matter-stripped
		// files; line 1 is the correct fixed anchor for a
		// whole-document projection error.
		Line:     1,
		RuleID:   "MDS020",
		Severity: lint.Error,
		Message:  d.Format(),
	})
}
