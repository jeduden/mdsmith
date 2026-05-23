package index

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"
)

// pooledParser pairs a parser.Parser with the reset closure that
// clears the link-ref transformer's pinned document source bytes.
// Returning the parser to parserPool without Reset would keep the
// last parsed file's []byte alive for the lifetime of the pool slot;
// the LSP and parallel index builds rotate through many large files,
// so the retention quickly compounds.
type pooledParser struct {
	parser parser.Parser
	reset  func()
}

// parserPool reuses goldmark parsers across buildFileEntry calls.
// lint.NewParser() builds a substantial config (block parsers, inline
// parsers, paragraph transformers); constructing one per file
// dominated the parallel-build wall-clock budget. parser.Parser
// instances are safe to reuse sequentially within a single goroutine.
var parserPool = sync.Pool{
	New: func() any {
		p, reset := lint.NewPooledParser()
		return &pooledParser{parser: p, reset: reset}
	},
}

// buildFileEntry parses source under filePath (workspace-relative) and
// extracts the symbol/edge tables for that file.
//
// Markdown link / directive parsing is delegated to the linkgraph
// package so the LSP graph, MDS027, and `mdsmith list backlinks`
// agree on what counts as a link, an anchor, and a directive target.
// The symbol-table collectors (headings, link-ref defs, directive
// outline entries) still live in this package because they're index-
// specific.
//
// The function is pure given its inputs: no file reads, no workspace
// traversal, no shared mutable state. Callers may invoke it
// concurrently across files.
func buildFileEntry(filePath string, source []byte) *FileEntry {
	fe := &FileEntry{
		Path:      NormalizePath(filePath),
		LineCount: countLines(source),
	}

	// Front matter is parsed first because it carries the file's
	// title / kinds — both surfaced as workspace-symbol matches.
	// frontMatterAll walks the YAML mapping once and surfaces the
	// outline keys, title, and kinds in a single pass to keep the
	// per-file allocation budget low (this was a measurable
	// bottleneck under parallel Build).
	fmBytes, body := lint.StripFrontMatter(source)
	fmOffset := countLines(fmBytes)
	fmSyms, fmTitle, fmKinds := frontMatterAll(fe.Path, fmBytes)
	fe.Symbols = append(fe.Symbols, fmSyms...)
	fe.Title = fmTitle
	fe.Kinds = fmKinds

	// Parse the body with the same goldmark configuration the lint
	// pipeline uses, so processing-instructions surface as our
	// custom AST node. The parser context carries the reference
	// definitions; collectLinkRefDefs reads from it directly.
	// Pull a parser out of the pool — building one is expensive
	// compared to a single parse. defer Put so a panic inside
	// Parse (or anywhere below) doesn't leak the instance.
	// Reset before Put so the link-ref transformer doesn't pin
	// the file's source bytes in the idle pool slot.
	pp := parserPool.Get().(*pooledParser)
	defer func() {
		pp.reset()
		parserPool.Put(pp)
	}()
	ctx := parser.NewContext()
	root := pp.parser.Parse(text.NewReader(body), parser.WithContext(ctx))
	lines := bytes.Split(body, []byte("\n"))

	// Wrap the parsed body in a *lint.File so the linkgraph
	// extractors (ExtractLinks / ExtractRefLinks / ExtractDirectives)
	// can use their f.LineOfOffset / f.ColumnOfOffset helpers.
	// LineOffset is set so callers that want file-relative coordinates
	// (the symbol layer, which adds fmOffset back) stay consistent.
	lf := &lint.File{
		Path:       fe.Path,
		Source:     body,
		Lines:      lines,
		AST:        root,
		LineOffset: fmOffset,
	}

	// Headings drive the outline.
	headingSyms := collectHeadings(fe.Path, root, body, lines, fmOffset, fe.LineCount)
	fe.Symbols = append(fe.Symbols, headingSyms...)

	// Link reference definitions (parsed by goldmark) flatten alongside.
	fe.Symbols = append(fe.Symbols, collectLinkRefDefs(fe.Path, ctx, body, lines, fmOffset)...)

	// Directives (PIs) at the document root.
	fe.Symbols = append(fe.Symbols, collectDirectives(fe.Path, root, body, fmOffset)...)

	// Edges: anchor / file / ref-style links plus directive targets.
	fe.Outgoing = append(fe.Outgoing, collectLinkEdges(fe.Path, lf, fmOffset)...)
	fe.Outgoing = append(fe.Outgoing, collectDirectiveEdges(fe.Path, lf, fmOffset)...)

	return fe
}

func countLines(source []byte) int {
	if len(source) == 0 {
		return 0
	}
	n := bytes.Count(source, []byte{'\n'})
	if source[len(source)-1] != '\n' {
		n++
	}
	return n
}

// collectHeadings returns one Symbol per heading. Range extends to
// the line before the next heading at the same or lower level — that
// matches how outline UIs (VS Code's symbol picker, Helix's
// jump-to-symbol) shade the section.
func collectHeadings(filePath string, root ast.Node, source []byte, lines [][]byte, fmOffset, totalLines int) []Symbol {
	heads, headStart := walkHeadings(root, source)
	syms := make([]Symbol, 0, len(heads))
	usedAnchors := make(map[string]bool)
	slugCounts := make(map[string]int)
	for i, h := range heads {
		txt := mdtext.ExtractPlainText(h, source)
		anchor := uniqueAnchor(mdtext.Slugify(txt), usedAnchors, slugCounts)
		startLine := headStart[i] + fmOffset
		endLine := headingEndLine(heads, headStart, i, fmOffset, totalLines)
		col := columnOfLine(lines, headStart[i]-1, h.Lines().At(0).Start, source)
		syms = append(syms, Symbol{
			File:          filePath,
			Kind:          SymbolHeading,
			Name:          txt,
			Anchor:        anchor,
			Level:         h.Level,
			StartLine:     startLine,
			EndLine:       endLine,
			SelectionLine: startLine,
			SelectionCol:  col,
		})
	}
	return syms
}

// walkHeadings collects every ast.Heading in document order along
// with its 1-based source line. Goldmark guarantees a parsed
// heading has at least one source line; setext-style headings
// also produce non-empty Lines().
func walkHeadings(root ast.Node, source []byte) ([]*ast.Heading, []int) {
	var heads []*ast.Heading
	var headStart []int
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		heads = append(heads, h)
		headStart = append(headStart, lineOfOffset(source, h.Lines().At(0).Start))
		return ast.WalkContinue, nil
	})
	return heads, headStart
}

// uniqueAnchor returns a slug suffixed with -1, -2, … when the bare
// slug is already used, mirroring CommonMark / GitHub disambiguation.
func uniqueAnchor(slug string, used map[string]bool, counts map[string]int) string {
	if slug == "" {
		return ""
	}
	anchor := slug
	if used[anchor] {
		c := counts[slug]
		for {
			c++
			anchor = fmt.Sprintf("%s-%d", slug, c)
			if !used[anchor] {
				break
			}
		}
		counts[slug] = c
	}
	used[anchor] = true
	return anchor
}

// headingEndLine returns the 1-based last line that belongs to
// heads[i]'s section: the line before the next heading at the same
// or lower level, clamped to totalLines.
func headingEndLine(heads []*ast.Heading, headStart []int, i, fmOffset, totalLines int) int {
	startLine := headStart[i] + fmOffset
	endLine := totalLines
	for j := i + 1; j < len(heads); j++ {
		if heads[j].Level <= heads[i].Level {
			endLine = headStart[j] - 1 + fmOffset
			break
		}
	}
	if endLine < startLine {
		endLine = startLine
	}
	return endLine
}

// columnOfLine returns the 1-based column of an absolute byte offset
// within a body parsed without the front matter. lines is bytes.Split
// of the same body.
func columnOfLine(lines [][]byte, lineIdx int, absOffset int, source []byte) int {
	if lineIdx < 0 || lineIdx >= len(lines) {
		return 1
	}
	// Compute cumulative offset to start of lineIdx.
	cum := 0
	for i := 0; i < lineIdx; i++ {
		cum += len(lines[i]) + 1 // +1 for the \n
	}
	if absOffset < cum {
		return 1
	}
	if absOffset > cum+len(lines[lineIdx]) {
		absOffset = cum + len(lines[lineIdx])
	}
	return absOffset - cum + 1
}

// lineOfOffset is a 1-based line index for a byte offset in source.
func lineOfOffset(source []byte, offset int) int {
	if offset < 0 {
		return 1
	}
	if offset > len(source) {
		offset = len(source)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

// frontMatterAll walks the front-matter YAML once and returns the
// per-key outline symbols, the title scalar, and the kinds list.
// Equivalent to calling frontMatterSymbols + frontMatterScalar(title)
// + frontMatterStringList(kinds) but parses the YAML body only one
// time, which removed a measurable bottleneck under parallel Build.
//
// Parsing goes through yamlutil so the index never expands a YAML
// alias on user-controlled content — the rest of mdsmith treats
// every front-matter parse as a potential alias-bomb vector and the
// symbol index has to match.
func frontMatterAll(filePath string, fm []byte) (syms []Symbol, title string, kinds []string) {
	if len(fm) == 0 {
		return nil, "", nil
	}
	node, err := yamlutil.UnmarshalNodeSafe(stripDelimiters(fm))
	if err != nil || len(node.Content) == 0 {
		return nil, "", nil
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, "", nil
	}
	syms = make([]Symbol, 0, len(mapping.Content)/2)
	for i := 0; i < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		v := mapping.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.Value == "" {
			continue
		}
		syms = append(syms, frontMatterKeySymbol(filePath, k))
		switch k.Value {
		case "title":
			if v.Kind == yaml.ScalarNode {
				title = v.Value
			}
		case "kinds":
			kinds = append(kinds, frontMatterKindsList(v)...)
		}
	}
	return syms, title, kinds
}

// frontMatterKeySymbol builds the SymbolFrontMatter entry for a YAML
// mapping key node. yaml.v3 line numbers are 1-based within the
// parsed buffer; the stripped buffer drops the leading "---" line
// so add 1 to recover file-relative coordinates.
func frontMatterKeySymbol(filePath string, k *yaml.Node) Symbol {
	return Symbol{
		File:          filePath,
		Kind:          SymbolFrontMatter,
		Name:          k.Value,
		StartLine:     k.Line + 1,
		EndLine:       k.Line + 1,
		SelectionLine: k.Line + 1,
		SelectionCol:  k.Column,
	}
}

// frontMatterKindsList extracts string entries from a `kinds:` block
// list value. yaml.v3 sets Tag to "!!str" for explicit strings and
// "" for unresolved plain scalars (which the type resolver maps to
// string when the value doesn't look like a number / bool). Anything
// tagged !!int, !!bool, !!float, etc. is filtered out — matches the
// previous map[string]any path that dropped them via type assertion.
func frontMatterKindsList(v *yaml.Node) []string {
	if v == nil || v.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]string, 0, len(v.Content))
	for _, item := range v.Content {
		if item.Kind != yaml.ScalarNode {
			continue
		}
		if item.Tag != "" && item.Tag != "!!str" {
			continue
		}
		out = append(out, item.Value)
	}
	return out
}

// frontMatterSymbols extracts top-level YAML keys from the front
// matter prefix and returns one Symbol per key. Kept exported-ish
// (package-private) for the targeted coverage test in
// coverage_test.go; the symbol-index build path now uses
// frontMatterAll for a single-pass parse.
func frontMatterSymbols(filePath string, fm []byte) []Symbol {
	if len(fm) == 0 {
		return nil
	}
	node, err := yamlutil.UnmarshalNodeSafe(stripDelimiters(fm))
	if err != nil || len(node.Content) == 0 {
		return nil
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]Symbol, 0, len(mapping.Content)/2)
	// yaml.v3 line numbers are 1-based within the parsed buffer; the
	// stripped buffer drops the leading "---" line so add 1.
	// Non-scalar keys (mapping or sequence keys per YAML spec) and
	// empty key values are skipped — they don't produce a sensible
	// outline entry and an empty Symbol.Name would render as a
	// blank row in the editor's outline.
	for i := 0; i < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Kind != yaml.ScalarNode || k.Value == "" {
			continue
		}
		out = append(out, Symbol{
			File:          filePath,
			Kind:          SymbolFrontMatter,
			Name:          k.Value,
			StartLine:     k.Line + 1,
			EndLine:       k.Line + 1,
			SelectionLine: k.Line + 1,
			SelectionCol:  k.Column,
		})
	}
	return out
}

// stripDelimiters removes the leading and trailing `---\n` lines
// from a front-matter prefix as returned by lint.StripFrontMatter.
// The trailing strip uses TrimSuffix with the exact `---\n`
// pattern (or `---` without a trailing newline as a fallback for
// truncated input) rather than scanning for the last occurrence
// of `---`. The previous LastIndex approach could match `---`
// inside YAML content (e.g. inside a multi-line quoted string),
// which would over-truncate the front matter.
func stripDelimiters(fm []byte) []byte {
	body := fm
	body = bytes.TrimPrefix(body, []byte("---\n"))
	if t := bytes.TrimSuffix(body, []byte("---\n")); len(t) != len(body) {
		return t
	}
	return bytes.TrimSuffix(body, []byte("---"))
}

// frontMatterScalar returns a top-level scalar key from front matter
// as a string. Empty string + false when absent or non-scalar.
// yamlutil.UnmarshalSafe rejects anchors/aliases so a malicious file
// can't trigger expansion during the symbol-index build. Non-string
// scalars (numbers, bools) are formatted via fmt.Sprintf so callers
// always get a stable string form.
func frontMatterScalar(fm []byte, key string) (string, bool) {
	if len(fm) == 0 {
		return "", false
	}
	var m map[string]any
	if err := yamlutil.UnmarshalSafe(stripDelimiters(fm), &m); err != nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return fmt.Sprintf("%v", v), true
}

// frontMatterStringList returns a top-level YAML list of strings.
// Parses via yamlutil so YAML aliases are rejected before any
// expansion can happen on the user's input.
func frontMatterStringList(fm []byte, key string) ([]string, bool) {
	if len(fm) == 0 {
		return nil, false
	}
	var m map[string]any
	if err := yamlutil.UnmarshalSafe(stripDelimiters(fm), &m); err != nil {
		return nil, false
	}
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out, true
}

// refDefRE matches a CommonMark reference definition at the start of
// a line. Cribbed from internal/rules/nounusedlinkdefinitions.
var refDefRE = regexp.MustCompile(`(?m)^[ ]{0,3}\[([^\]\n]+)\]:[ \t]*\S+.*$`)

// RefDefRegexpMatches returns the same submatch indices
// refDefRE.FindAllSubmatchIndex produces for body. Exported so the
// LSP rename surface can iterate every reference definition without
// duplicating the regex pattern (and without giving callers a way
// to mutate the package-level pattern).
func RefDefRegexpMatches(body []byte) [][]int {
	return refDefRE.FindAllSubmatchIndex(body, -1)
}

// collectLinkRefDefs finds `[label]: url` lines in body. The CommonMark
// reference definition map is stored in parser.Context (`ctx.References`)
// — we use it to confirm a regex match really is a definition (not a
// link inside a paragraph), then read the line/col from the source.
func collectLinkRefDefs(filePath string, ctx parser.Context, body []byte, lines [][]byte, fmOffset int) []Symbol {
	wanted := map[string]bool{}
	for _, ref := range ctx.References() {
		wanted[string(ref.Label())] = true
	}
	if len(wanted) == 0 {
		return nil
	}
	// Track normalized labels we've already emitted: goldmark
	// resolves only the first definition for any label, so duplicate
	// regex matches must not produce extra outline entries that
	// would confuse the symbol picker.
	seen := map[string]bool{}
	var out []Symbol
	for _, m := range refDefRE.FindAllSubmatchIndex(body, -1) {
		raw := body[m[2]:m[3]]
		label := string(raw)
		anchor := string(util.ToLinkReference(raw))
		if !wanted[anchor] || seen[anchor] {
			continue
		}
		seen[anchor] = true
		// m[2]-1 is the offset of `[`; m[2] is the offset of the
		// label's first byte. Use the label position so "go to
		// definition" highlights the label, not the bracket.
		labelOffset := m[2]
		line := lineOfOffset(body, labelOffset) + fmOffset
		col := columnOfLine(lines, lineOfOffset(body, labelOffset)-1, labelOffset, body)
		out = append(out, Symbol{
			File:          filePath,
			Kind:          SymbolLinkRef,
			Name:          label,
			Anchor:        anchor,
			StartLine:     line,
			EndLine:       line,
			SelectionLine: line,
			SelectionCol:  col,
		})
	}
	return out
}

// collectDirectives returns one Symbol per processing-instruction
// block at the document root. Closing markers (<?/name?>) are
// skipped; only the opener is treated as a symbol.
func collectDirectives(filePath string, root ast.Node, source []byte, fmOffset int) []Symbol {
	var out []Symbol
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*lint.ProcessingInstruction)
		if !ok {
			continue
		}
		if strings.HasPrefix(pi.Name, "/") {
			continue
		}
		startLine, endLine := piLineRange(pi, source, fmOffset)
		out = append(out, Symbol{
			File:          filePath,
			Kind:          SymbolDirective,
			Name:          pi.Name,
			StartLine:     startLine,
			EndLine:       endLine,
			SelectionLine: startLine,
			SelectionCol:  1,
		})
	}
	return out
}

// piLineRange returns the 1-based [start, end] source lines for a
// processing-instruction block. The PI parser guarantees Lines() is
// non-empty for any parsed PI, so the helper does not handle that
// case. The closing-marker offset (`?>`) on a continuation line
// gives the end; goldmark emits HasClosure() == true for every
// well-formed PI, so the branch where Lines() spans multiple
// segments without a closure is unreachable in practice.
func piLineRange(pi *lint.ProcessingInstruction, source []byte, fmOffset int) (int, int) {
	startSeg := pi.Lines().At(0)
	startLine := lineOfOffset(source, startSeg.Start) + fmOffset
	endLine := startLine
	if pi.HasClosure() && pi.ClosureLine.Start > startSeg.Start {
		endLine = lineOfOffset(source, pi.ClosureLine.Start) + fmOffset
	}
	return startLine, endLine
}

// collectLinkEdges emits one Edge per Markdown link in f. Inline
// links produce EdgeAnchorLink (`[x](#sec)`) or EdgeFileLink
// (`[x](./other.md)`); reference-style links (`[x][label]`) produce
// EdgeRefLink. Extraction routes through linkgraph so MDS027, the
// backlinks CLI, and this index walk the same parser.
//
// Absolute / escapes-the-root file targets are dropped (linkgraph's
// ResolveRelTarget returns ""). Anchor slugs are normalised via
// linkgraph.NormalizeAnchor so a percent-encoded `%2D` keys to the
// same slot as the literal `-`.
//
// Lines and columns are file-relative (body line + fmOffset) so
// downstream LSP locations don't need to adjust for front matter.
func collectLinkEdges(filePath string, f *lint.File, fmOffset int) []Edge {
	var out []Edge
	for _, l := range linkgraph.ExtractLinks(f) {
		anchor := linkgraph.NormalizeAnchor(l.Target.Anchor)
		if l.Target.LocalAnchor {
			out = append(out, Edge{
				SourceFile:   filePath,
				SourceLine:   l.Line + fmOffset,
				SourceCol:    l.Column,
				TargetAnchor: anchor,
				Kind:         EdgeAnchorLink,
			})
			continue
		}
		tgt := linkgraph.ResolveRelTarget(filePath, l.Target.Path)
		if tgt == "" {
			// Absolute or escapes-the-root paths cannot point at
			// anything inside the workspace. Emitting an edge with
			// empty TargetFile would be ambiguous — IncomingEdges
			// treats `""` as "same file as source", so the link
			// would be misattributed as a self-reference. Drop it.
			continue
		}
		out = append(out, Edge{
			SourceFile:   filePath,
			SourceLine:   l.Line + fmOffset,
			SourceCol:    l.Column,
			TargetFile:   tgt,
			TargetAnchor: anchor,
			Kind:         EdgeFileLink,
		})
	}
	for _, r := range linkgraph.ExtractRefLinks(f) {
		out = append(out, Edge{
			SourceFile:  filePath,
			SourceLine:  r.Line + fmOffset,
			SourceCol:   r.Column,
			TargetLabel: r.Label,
			Kind:        EdgeRefLink,
		})
	}
	return out
}

// collectDirectiveEdges emits one Edge per `<?include?>`,
// `<?catalog?>`, and `<?build?>` directive whose body specifies a
// usable target. Include and build edges carry a workspace-relative
// TargetFile. Catalog edges are emitted with Unresolved=true and an
// empty TargetFile — the glob list isn't expanded inside the
// per-file extractor (see linkgraph.ExpandCatalog for callers that
// need the concrete list), and IncomingEdges skips unresolved edges
// so catalog hosts don't appear as phantom self-backlinks.
//
// Targets that are absolute or escape the workspace are dropped
// silently; dedicated lint rules report those as diagnostics.
func collectDirectiveEdges(filePath string, f *lint.File, fmOffset int) []Edge {
	var out []Edge
	for _, d := range linkgraph.ExtractDirectives(f) {
		line := d.Line + fmOffset
		switch d.Kind {
		case linkgraph.DirectiveInclude:
			tgt := linkgraph.ResolveRelTarget(filePath, d.Path)
			if tgt == "" {
				continue
			}
			out = append(out, Edge{
				SourceFile: filePath,
				SourceLine: line,
				SourceCol:  d.Col,
				TargetFile: tgt,
				Kind:       EdgeInclude,
			})
		case linkgraph.DirectiveBuild:
			tgt := linkgraph.ResolveRelTarget(filePath, d.Path)
			if tgt == "" {
				continue
			}
			out = append(out, Edge{
				SourceFile: filePath,
				SourceLine: line,
				SourceCol:  d.Col,
				TargetFile: tgt,
				Kind:       EdgeBuild,
			})
		case linkgraph.DirectiveCatalog:
			out = append(out, Edge{
				SourceFile: filePath,
				SourceLine: line,
				SourceCol:  d.Col,
				Kind:       EdgeCatalog,
				Unresolved: true,
			})
		}
	}
	return out
}
