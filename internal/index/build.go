package index

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
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
	// Built once and shared by every line/column lookup below
	// (collectHeadings, collectLinkRefDefs, collectDirectives) instead
	// of each rebuilding its own — same source, same index. See
	// docs/development/high-performance-go.md, "Memoize per-input
	// computations".
	nl := newlineIndex(body)

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
	headingSyms := collectHeadings(fe.Path, root, body, lines, nl, fmOffset, fe.LineCount)
	fe.Symbols = append(fe.Symbols, headingSyms...)

	// Link reference definitions (parsed by goldmark) flatten alongside.
	fe.Symbols = append(fe.Symbols, collectLinkRefDefs(fe.Path, ctx, body, lines, nl, fmOffset)...)

	// Directives (PIs) at the document root.
	fe.Symbols = append(fe.Symbols, collectDirectives(fe.Path, root, nl, fmOffset)...)

	// Edges: anchor / file / ref-style links plus directive targets,
	// then Obsidian-style wikilinks (keyed by stem for the move planner).
	fe.Outgoing = append(fe.Outgoing, collectLinkEdges(fe.Path, lf, fmOffset)...)
	fe.Outgoing = append(fe.Outgoing, collectDirectiveEdges(fe.Path, lf, fmOffset)...)
	fe.Outgoing = append(fe.Outgoing, collectWikilinkEdges(fe.Path, lf, fmOffset)...)

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
func collectHeadings(
	filePath string, root ast.Node, source []byte, lines [][]byte, nl []int, fmOffset, totalLines int,
) []Symbol {
	heads, headStart := walkHeadings(root, nl)
	syms := make([]Symbol, 0, len(heads))
	usedAnchors := make(map[string]struct{})
	slugCounts := make(map[string]int)
	for i, h := range heads {
		txt := mdtext.ExtractPlainText(h, source)
		anchor := uniqueAnchor(mdtext.Slugify(txt), usedAnchors, slugCounts)
		startLine := headStart[i] + fmOffset
		endLine := headingEndLine(heads, headStart, i, fmOffset, totalLines)
		col := columnOfLineIndexed(nl, lines, headStart[i]-1, h.Lines().At(0).Start)
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
func walkHeadings(root ast.Node, nl []int) ([]*ast.Heading, []int) {
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
		headStart = append(headStart, lineOfOffsetIndexed(nl, h.Lines().At(0).Start))
		return ast.WalkContinue, nil
	})
	return heads, headStart
}

// uniqueAnchor returns a slug suffixed with -1, -2, … when the bare
// slug is already used, mirroring CommonMark / GitHub disambiguation.
func uniqueAnchor(slug string, used map[string]struct{}, counts map[string]int) string {
	if slug == "" {
		return ""
	}
	anchor := slug
	if _, ok := used[anchor]; ok {
		c := counts[slug]
		for {
			c++
			anchor = slug + "-" + strconv.Itoa(c)
			if _, ok := used[anchor]; !ok {
				break
			}
		}
		counts[slug] = c
	}
	used[anchor] = struct{}{}
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
	// bytes.Count special-cases a one-byte separator to a SIMD byte count;
	// a hand-rolled scan loop is not vectorized.
	return 1 + bytes.Count(source[:offset], []byte{'\n'})
}

// frontMatterAll walks the front-matter YAML once and returns the
// per-key outline symbols, the title scalar, and the kinds list.
// The outline and the title come from one linear walk of the parsed
// node, which removed a measurable bottleneck under parallel Build.
//
// Parsing goes through yamlutil so the index never expands a YAML
// alias on user-controlled content — the rest of mdsmith treats
// every front-matter parse as a potential alias-bomb vector and the
// symbol index has to match.
//
// A title key that appears twice has no single value, so it yields
// no title. A title supplied only through a merge (`<<`) key is
// read from the parsed node by mergedTitle. Kinds come from
// lint.FrontMatterKindsFromNode, the engine's own decode applied
// to the node parsed here, so the index
// never reports a kind the engine would not apply and the YAML is
// parsed only once. It runs only when the walk saw a `kinds` or
// merge (`<<`) key, the only keys that can carry kinds.
func frontMatterAll(filePath string, fm []byte) (syms []Symbol, title string, kinds []string) {
	if len(fm) == 0 {
		return nil, "", nil
	}
	body := lint.FrontMatterYAML(fm)
	node, err := yamlutil.UnmarshalNodeSafe(body)
	if err != nil || len(node.Content) == 0 {
		return nil, "", nil
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, "", nil
	}
	syms = make([]Symbol, 0, len(mapping.Content)/2)
	titles, sawMerge, mayHaveKinds := 0, false, false
	for i := 0; i < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Kind != yaml.ScalarNode || k.Value == "" {
			continue
		}
		syms = append(syms, frontMatterKeySymbol(filePath, k))
		switch k.Value {
		case "title":
			titles++
			title = frontMatterTitle(mapping.Content[i+1])
		case "<<":
			sawMerge, mayHaveKinds = true, true
		case "kinds":
			mayHaveKinds = true
		}
	}
	switch {
	case titles > 1:
		title = ""
	case titles == 0 && sawMerge:
		title = mergedTitle(&node)
	}
	if mayHaveKinds {
		kinds, _ = lint.FrontMatterKindsFromNode(body, &node)
	}
	return syms, title, kinds
}

// mergedTitle returns the title a merge (`<<`) key supplies when the
// mapping has no `title:` key of its own. It decodes the parsed
// document node, so the merge follows the engine's map decode: the
// first merged mapping that sets title wins. A decode error, such as
// a duplicate key, yields no title, as it does for the engine.
func mergedTitle(doc *yaml.Node) string {
	var fm struct {
		Title yaml.Node `yaml:"title"`
	}
	if yamlutil.DecodeNodeSafe(doc, &fm) != nil || fm.Title.Kind == 0 {
		return ""
	}
	return frontMatterTitle(&fm.Title)
}

// frontMatterTitle returns the display text of a `title:` value
// node. An absent, null, or non-scalar value has no title. Runs of
// Unicode whitespace, including the newlines a block scalar
// carries, collapse to one space and the ends are trimmed, so the
// title fits on a single workspace-symbol row. Typed scalars
// (numbers, booleans, dates) keep their source spelling. A
// `!!binary` value shows its decoded text, as the engine's map
// decode sees it; one that is not valid base64 or not UTF-8 text
// has no title.
func frontMatterTitle(v *yaml.Node) string {
	if v.Kind != yaml.ScalarNode || v.Tag == "!!null" {
		return ""
	}
	text := v.Value
	if v.Tag == "!!binary" {
		var decoded string
		if yamlutil.DecodeNodeSafe(v, &decoded) != nil || !utf8.ValidString(decoded) {
			return ""
		}
		text = decoded
	}
	if !needsSpaceCollapse(text) {
		return text
	}
	return strings.Join(strings.Fields(text), " ")
}

// needsSpaceCollapse reports whether s has leading or trailing
// whitespace, two whitespace runes in a row, or any whitespace rune
// other than an ASCII space. Most titles need none of these, so
// frontMatterTitle skips the Fields/Join allocations for them.
func needsSpaceCollapse(s string) bool {
	prevSpace := true // a leading space counts as a run
	for _, r := range s {
		sp := unicode.IsSpace(r)
		if sp && (r != ' ' || prevSpace) {
			return true
		}
		prevSpace = sp
	}
	return prevSpace && s != ""
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
func collectLinkRefDefs(
	filePath string, ctx parser.Context, body []byte, lines [][]byte, nl []int, fmOffset int,
) []Symbol {
	refs := ctx.References()
	wanted := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		wanted[string(util.ToLinkReference(ref.Label()))] = struct{}{}
	}
	if len(wanted) == 0 {
		return nil
	}
	// Track normalized labels we've already emitted: goldmark
	// resolves only the first definition for any label, so duplicate
	// regex matches must not produce extra outline entries that
	// would confuse the symbol picker.
	seen := make(map[string]struct{}, len(wanted))
	out := make([]Symbol, 0, len(wanted))
	for _, m := range refDefRE.FindAllSubmatchIndex(body, -1) {
		raw := body[m[2]:m[3]]
		label := string(raw)
		anchor := string(util.ToLinkReference(raw))
		_, inWanted := wanted[anchor]
		_, inSeen := seen[anchor]
		if !inWanted || inSeen {
			continue
		}
		seen[anchor] = struct{}{}
		// m[2]-1 is the offset of `[`; m[2] is the offset of the
		// label's first byte. Use the label position so "go to
		// definition" highlights the label, not the bracket.
		labelOffset := m[2]
		lineIdx := lineOfOffsetIndexed(nl, labelOffset)
		line := lineIdx + fmOffset
		col := columnOfLineIndexed(nl, lines, lineIdx-1, labelOffset)
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
func collectDirectives(filePath string, root ast.Node, nl []int, fmOffset int) []Symbol {
	var out []Symbol
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*piparser.ProcessingInstruction)
		if !ok {
			continue
		}
		if strings.HasPrefix(pi.Name, "/") {
			continue
		}
		startLine, endLine := piLineRange(pi, nl, fmOffset)
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
func piLineRange(pi *piparser.ProcessingInstruction, nl []int, fmOffset int) (int, int) {
	startSeg := pi.Lines().At(0)
	startLine := lineOfOffsetIndexed(nl, startSeg.Start) + fmOffset
	endLine := startLine
	if pi.HasClosure() && pi.ClosureLine.Start > startSeg.Start {
		endLine = lineOfOffsetIndexed(nl, pi.ClosureLine.Start) + fmOffset
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

// collectWikilinkEdges emits one EdgeWikilink per Obsidian-style
// `[[stem]]` link that resolves by Markdown basename stem. TargetLabel
// carries the lowercased stem (linkgraph.WikilinkStem, mirroring the
// resolver) and the edge is Unresolved with an empty TargetFile: stem
// resolution needs the whole workspace, so the concrete target is left
// to the move planner, which matches by stem. Typed non-Markdown
// embeds (`![[diagram.png]]`) resolve by exact filename, never point at
// a Markdown file, and are skipped.
func collectWikilinkEdges(filePath string, f *lint.File, fmOffset int) []Edge {
	var out []Edge
	for _, w := range linkgraph.ExtractWikiLinks(f) {
		stem, ok := linkgraph.WikilinkStem(w.Target)
		if !ok {
			continue
		}
		out = append(out, Edge{
			SourceFile:  filePath,
			SourceLine:  w.Line + fmOffset,
			SourceCol:   w.Column,
			TargetLabel: stem,
			Kind:        EdgeWikilink,
			Unresolved:  true,
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
			if d.IsUnresolved() {
				// A glob inputs: entry — emit an unresolved build edge,
				// the same shape as a catalog edge, so reverse-edge
				// queries skip it until the glob is expanded.
				out = append(out, Edge{
					SourceFile: filePath,
					SourceLine: line,
					SourceCol:  d.Col,
					Kind:       EdgeBuild,
					Unresolved: true,
				})
				continue
			}
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
