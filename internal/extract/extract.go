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
	// When the composed schema roots at H2, the document H1 is
	// outside every schema scope. Emit it under the reserved
	// "title" key beside `frontmatter`. No H1 → key omitted
	// (consistent with optional sections: omitted, not null).
	// First H1 wins when a document carries more than one.
	// Collision with a sibling scope that resolves to "title"
	// is reported through the existing setKey machinery.
	if sch.EffectiveRootLevel() == 2 {
		if h1Text := p.firstH1PlainText(); h1Text != "" {
			p.setKey(root, "title", h1Text)
		}
	}
	p.projectChildren(m.Root.Children, root)
	if len(p.diags) > 0 {
		return nil, p.diags
	}
	return root, nil
}

// firstH1PlainText returns the plain text of the first H1 heading
// found in the document, or an empty string if there is none.
// It uses ExtractPlainText (the same renderer the heading matcher
// and content projector use) so emphasis and code-span markers
// are stripped and link text is kept.
func (p *projector) firstH1PlainText() string {
	for _, dh := range schema.ExtractDocHeadings(p.f) {
		if dh.Level == 1 {
			return strings.TrimSpace(dh.Text)
		}
	}
	return ""
}

type projector struct {
	f     *lint.File
	sch   *schema.Schema
	diags []lint.Diagnostic
}

// keyFor is the single key-naming seam — the one function plan 167
// overrides through `bind:`. A non-empty `Bind` wins (the value
// replaces the default key). An empty bind is the hoist signal —
// projectChildren routes hoist groups through hoistGroup before
// reaching keyFor, but the empty-string check here keeps any
// future caller from accidentally writing a blank key. The
// fallback chain derives the default from the heading: a literal
// heading slugifies whole; a placeholder-bearing heading slugifies
// its literal stem, falling back to the first `fmvar` field name
// when the heading is only a placeholder (`## {id}`).
func keyFor(sc *schema.Scope) string {
	if sc != nil && sc.Bind != nil && *sc.Bind != "" {
		return *sc.Bind
	}
	stem, fmvars, _ := schema.HeadingStem(sc)
	if s := mdtext.Slugify(stem); s != "" {
		return s
	}
	if len(fmvars) > 0 {
		return fmvars[0]
	}
	return mdtext.Slugify(sc.Heading)
}

// inlineBlockParagraphs reports whether paragraph blocks in this
// scope's body render as inline spans. The scope's own
// `block-paragraphs` wins; an unset scope value (or a nil scope, the
// unlisted case) falls back to the schema-level default. Only an
// explicit `inline` turns it on. Plan 246.
func (p *projector) inlineBlockParagraphs(sc *schema.Scope) bool {
	if sc != nil && sc.BlockParagraphs != "" {
		return sc.BlockParagraphs == schema.ProjectionInline
	}
	return p.sch != nil && p.sch.BlockParagraphs == schema.ProjectionInline
}

// hoistsToParent reports whether sm is a scope match whose bind
// override directs it to be hoisted into the parent (`bind: ""`).
// A preamble has the same effect by definition; this helper covers
// the explicit bind form for non-preamble scopes.
func hoistsToParent(sm *schema.ScopeMatch) bool {
	if sm == nil || sm.Scope == nil {
		return false
	}
	return sm.Scope.Bind != nil && *sm.Scope.Bind == ""
}

// isRepeating reports whether a scope projects as an array. A
// declared `repeat:` cardinality is the signal; an unset matcher
// (exactly one) projects as a single object.
func isRepeating(sc *schema.Scope) bool {
	return sc != nil && sc.Matcher != nil && sc.Matcher.Repeat.Set
}

// projectChildren walks a contiguous list of sibling scope matches,
// grouping consecutive occurrences of the same schema scope, and
// writes each group's projection into obj. A preamble group — and a
// non-preamble group whose scope sets `bind: ""` — hoists its
// content directly into obj (no wrapper key).
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
		if sm.Unlisted {
			// Unlisted matches (schema-level `projection: blocks` only)
			// have a nil Scope, so they group by heading slug rather
			// than scope pointer. Hand the whole contiguous run to
			// projectUnlisted, which groups same-slug headings into
			// arrays. collectUnlistedBlockMatches appends them after
			// the declared matches, so the run reaches the end.
			j := i + 1
			for j < len(children) && children[j].Unlisted {
				j++
			}
			p.projectUnlisted(children[i:j], obj)
			i = j
			continue
		}
		j := i + 1
		for j < len(children) && children[j].Scope == sm.Scope {
			j++
		}
		group := children[i:j]
		i = j

		if hoistsToParent(sm) {
			p.hoistGroup(group, obj)
			continue
		}

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

// projectUnlisted projects a run of unlisted (synthetic) scope matches
// under a schema-level `projection: blocks`. Each projects as a
// `{heading, blocks}` object keyed by the slug of its heading text.
// Headings whose slugs collide project as an array under that key, so
// a section heading that repeats (`## Note`, `## Note`) survives
// without a collision diagnostic. Plan 246.
func (p *projector) projectUnlisted(
	group []*schema.ScopeMatch, obj map[string]any,
) {
	// Group by slug in first-seen order so the array elements keep
	// document order.
	bySlug := map[string][]*schema.ScopeMatch{}
	var order []string
	for _, sm := range group {
		key := mdtext.Slugify(sm.Heading.Text)
		if _, seen := bySlug[key]; !seen {
			order = append(order, key)
		}
		bySlug[key] = append(bySlug[key], sm)
	}
	for _, key := range order {
		ms := bySlug[key]
		if len(ms) == 1 {
			p.setKey(obj, key, p.projectUnlistedObject(ms[0]))
			continue
		}
		arr := make([]any, 0, len(ms))
		for _, sm := range ms {
			arr = append(arr, p.projectUnlistedObject(sm))
		}
		p.setKey(obj, key, arr)
	}
}

// projectUnlistedObject builds the object for one unlisted section: a
// `heading` text field (preserving the original heading text the slug
// key flattens) plus the body `blocks` list. projectScopeObject adds
// the `blocks` key from sm.Body.
func (p *projector) projectUnlistedObject(sm *schema.ScopeMatch) map[string]any {
	obj := p.projectScopeObject(sm)
	p.setKey(obj, "heading", sm.Heading.Text)
	return obj
}

// hoistGroup merges every element of group directly into obj
// instead of nesting under a key. A `bind: ""` scope's projection
// is its own child-scopes and content, so projectScopeObject's
// captures, children, and content all surface as siblings of obj's
// existing keys; collisions go through setKey like any other write.
// A repeating hoisted scope would silently overwrite — same shape
// as duplicate headings on a non-repeating bind, so we flag it.
func (p *projector) hoistGroup(group []*schema.ScopeMatch, obj map[string]any) {
	if len(group) > 1 {
		p.collision("<hoist>",
			"a repeating scope cannot hoist (`bind: \"\"`) because "+
				"multiple occurrences would overwrite each other")
		return
	}
	for k, v := range p.projectScopeObject(group[0]) {
		p.setKey(obj, k, v)
	}
}

// projectScopeObject builds the object value for one matched scope:
// its captured placeholders (name: value), then its child scopes,
// then its content entries, then — when the scope (or the schema)
// projects `blocks` — its whole body under a `blocks` key.
func (p *projector) projectScopeObject(sm *schema.ScopeMatch) map[string]any {
	obj := map[string]any{}
	for name, val := range sm.Captures {
		p.setKey(obj, name, val)
	}
	p.projectChildren(sm.Children, obj)
	p.projectContent(sm.Content, obj)
	// The whole-body `blocks` list (plan 246) joins the default-key
	// set: setKey routes it through the same collision check, so a
	// declared content entry that binds to `blocks` is reported rather
	// than silently overwritten. An empty section still emits
	// `blocks: []` (keyed on ProjectsBlocks, not len(Body)) for a
	// stable shape. The paragraph rendering choice (scope wins over
	// schema default) is passed down the walk, never stored, so no
	// state survives this projection; nested sections and quotes
	// inherit the same choice.
	if sm.ProjectsBlocks {
		p.setKey(obj, "blocks",
			p.blocksFromNodes(sm.Body, p.inlineBlockParagraphs(sm.Scope)))
	}
	return obj
}

// projectContent projects code-block, list, table, and paragraph
// entries under their default keys. Repeated kinds get a -N suffix
// (code, code-2, …) so a second block never silently overwrites
// the first. A non-nil `bind:` on the entry overrides the default
// base name; the same -N collision-suffix logic still applies so
// two entries that bind to the same name disambiguate.
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
		base := contentBaseKey(cm.Entry)
		switch cm.Entry.Kind {
		case schema.ContentKindCodeBlock:
			p.setKey(obj, nextKey(base), p.codeBody(cm.Node))
		case schema.ContentKindList:
			if cm.Entry.Projection == schema.ProjectionTree {
				p.setKey(obj, nextKey(base), p.listTree(cm.Node))
			} else {
				p.setKey(obj, nextKey(base), p.listItems(cm.Node))
			}
		case schema.ContentKindTable:
			if cm.Entry.Projection == schema.ProjectionRows {
				// `rows` projection injects two sibling keys —
				// `columns` and `rows` — directly into the section
				// object so the consumer sees them as peers, matching
				// the {"columns":[...], "rows":[[...]...]} shape the
				// plan defines. The base key is not used here; the
				// two key names are fixed by the projection spec.
				cols, rowArrays := p.tableRowsPositional(cm.Node)
				p.setKey(obj, "columns", cols)
				p.setKey(obj, "rows", rowArrays)
			} else {
				p.setKey(obj, nextKey(base), p.tableRows(cm.Node))
			}
		case schema.ContentKindParagraph:
			if cm.Entry.Projection == schema.ProjectionInline {
				// Resolve the key once so the unsupported-inline
				// diagnostic can name the same key setKey writes under.
				key := nextKey(base)
				p.setKey(obj, key, p.inlineSpans(key, cm.Node))
			} else {
				p.setKey(obj, nextKey(base), p.nodeText(cm.Node))
			}
		}
	}
}

// contentBaseKey returns the base projection key for a content
// entry: the user-supplied bind value when set, otherwise the
// kind's default name (`code`/`inline`/`items`/`rows`/`text`). A
// paragraph projected as inline spans defaults to `inline` instead of
// `text`, so a scope with both a text paragraph and an inline
// paragraph gives them distinct default keys (content entries are
// positional, each binds its own node) instead of colliding on `text`
// (plan 212).
func contentBaseKey(e *schema.ContentEntry) string {
	if e.Bind != nil {
		return *e.Bind
	}
	switch e.Kind {
	case schema.ContentKindCodeBlock:
		return "code"
	case schema.ContentKindList:
		return "items"
	case schema.ContentKindTable:
		return "rows"
	case schema.ContentKindParagraph:
		if e.Projection == schema.ProjectionInline {
			return "inline"
		}
		return "text"
	}
	return ""
}

func (p *projector) codeBody(n ast.Node) string {
	fcb, ok := n.(*ast.FencedCodeBlock)
	if !ok {
		return ""
	}
	return strings.TrimRight(p.rawLines(fcb), "\n")
}

func (p *projector) listItems(n ast.Node) []any {
	lst, ok := n.(*ast.List)
	if !ok {
		return nil
	}
	var items []any
	for c := lst.FirstChild(); c != nil; c = c.NextSibling() {
		items = append(items, p.itemOwnText(c))
	}
	return items
}

// itemOwnText returns a list item's own inline text — the text of its
// direct block children, with any nested sub-list excluded. The bare
// mdtext.ExtractPlainText recursion would splice a child item's text
// into the parent with no separator (`boldnested child`), corrupting
// the data; restricting it to non-List blocks keeps each flat string
// to the item it belongs to. An item whose only content is a nested
// list has no own text and projects as the empty string, preserving
// the item's position in the array. Task markers (`[x]` / `[ ]`) are
// left verbatim in flat mode, matching the historical output.
func (p *projector) itemOwnText(item ast.Node) string {
	var b strings.Builder
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*ast.List); ok {
			continue
		}
		b.WriteString(mdtext.ExtractPlainText(c, p.f.Source))
	}
	return strings.TrimSpace(b.String())
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

// tableRowsPositional implements the `projection: rows` walker. It
// returns two values:
//
//   - cols: a []any of column header strings in document order.
//   - rows: a []any of []any row arrays, one per body row; each element
//     is a string. Short rows are padded with empty strings to match
//     the header width.
//
// Duplicate headers are accepted — the columns array is positional, so
// there is no key collision. Plan 245.
//
// A table node with a header but no body rows returns rows as a
// non-nil empty slice (not nil): under `projection: blocks` the result
// serialises to `"rows": []`, which the published CUE contract's
// `rows: [...[...string]]` accepts, whereas a nil slice would serialise
// to `"rows": null` and fail validation. The nil return above stays
// reserved for the not-a-table guard.
func (p *projector) tableRowsPositional(n ast.Node) (cols []any, rows []any) {
	tbl, ok := n.(*extast.Table)
	if !ok {
		return nil, nil
	}
	// Both halves start non-nil for the same contract reason: a table
	// node without a TableHeader child (hand-built; the GFM parser
	// always emits one) must serialise `"columns": []`, not null.
	cols = []any{}
	rows = []any{}
	var colCount int
	for r := tbl.FirstChild(); r != nil; r = r.NextSibling() {
		var cells []string
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			cells = append(cells, strings.TrimSpace(
				mdtext.ExtractPlainText(c, p.f.Source)))
		}
		if _, isHeader := r.(*extast.TableHeader); isHeader {
			colCount = len(cells)
			cols = make([]any, len(cells))
			for i, h := range cells {
				cols[i] = h
			}
			continue
		}
		// Pad short body rows with empty strings.
		row := make([]any, colCount)
		for i := 0; i < colCount; i++ {
			if i < len(cells) {
				row[i] = cells[i]
			} else {
				row[i] = ""
			}
		}
		rows = append(rows, row)
	}
	return cols, rows
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
	p.emit(schema.SchemaDiagnostic{
		Field:     key,
		Actual:    "<collision>",
		Expected:  "a unique projection key",
		Hint:      why,
		SchemaRef: schema.FormatSchemaRef(p.sch, ""),
	})
}

// emit appends a SchemaDiagnostic as an MDS020 error. Extract returns
// its diagnostics straight to the CLI formatter without running them
// through lint.File.AdjustDiagnostics, so the line must already be an
// absolute file line. schema.NonBodyDiagLine returns 1-LineOffset
// (meant for later adjustment) and would print a zero/negative line
// for front-matter-stripped files; line 1 is the correct fixed anchor
// for a whole-document projection error.
//
// Route through Emit (rather than building the Diagnostic by hand) so
// the schema reference rides on a RelatedLocation like every other
// MDS020 emit site — Format() no longer carries it (plan 230).
func (p *projector) emit(d schema.SchemaDiagnostic) {
	mk := func(file string, line int, msg string) lint.Diagnostic {
		return lint.Diagnostic{
			File:     file,
			Line:     line,
			RuleID:   "MDS020",
			Severity: lint.Error,
			Message:  msg,
		}
	}
	p.diags = append(p.diags, d.Emit(mk, p.f.Path, 1))
}
