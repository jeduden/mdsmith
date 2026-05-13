package index

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"
)

// buildFileEntry parses source under filePath (workspace-relative) and
// extracts the symbol/edge tables for that file.
func buildFileEntry(filePath string, source []byte) *FileEntry {
	fe := &FileEntry{
		Path:      NormalizePath(filePath),
		LineCount: countLines(source),
	}

	// Front matter is parsed first because it carries the file's
	// title / kinds — both surfaced as workspace-symbol matches.
	fmBytes, body := lint.StripFrontMatter(source)
	fmOffset := countLines(fmBytes)
	fe.Symbols = append(fe.Symbols, frontMatterSymbols(fe.Path, fmBytes)...)
	if title, ok := frontMatterScalar(fmBytes, "title"); ok {
		fe.Title = title
	}
	if kinds, ok := frontMatterStringList(fmBytes, "kinds"); ok {
		fe.Kinds = kinds
	}

	// Parse the body with the same goldmark configuration the lint
	// pipeline uses, so processing-instructions surface as our
	// custom AST node.
	ctx := parser.NewContext()
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(ctx))
	lines := bytes.Split(body, []byte("\n"))

	// Headings drive the outline.
	headingSyms := collectHeadings(fe.Path, root, body, lines, fmOffset, fe.LineCount)
	fe.Symbols = append(fe.Symbols, headingSyms...)

	// Link reference definitions (parsed by goldmark) flatten alongside.
	fe.Symbols = append(fe.Symbols, collectLinkRefDefs(fe.Path, ctx, body, lines, fmOffset)...)

	// Directives (PIs) at the document root.
	fe.Symbols = append(fe.Symbols, collectDirectives(fe.Path, root, body, fmOffset)...)

	// Edges: anchor / file / ref-style links plus directive targets.
	fe.Outgoing = append(fe.Outgoing, collectLinkEdges(fe.Path, root, body, fmOffset)...)
	fe.Outgoing = append(fe.Outgoing, collectDirectiveEdges(fe.Path, root, body, fmOffset)...)

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

// frontMatterSymbols extracts top-level YAML keys from the front
// matter prefix and returns one Symbol per key. Lines are 1-based
// from the start of the file. Parsing goes through yamlutil so the
// index never expands a YAML alias on user-controlled content — the
// rest of mdsmith treats every front-matter parse as a potential
// alias-bomb vector and the symbol index has to match.
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

// collectLinkEdges walks the AST for ast.Link nodes and emits one
// Edge per parsed destination. Anchor-only links (`#fragment`) become
// EdgeAnchorLink; reference-style links (`[text][label]`) become
// EdgeRefLink; everything else becomes EdgeFileLink.
func collectLinkEdges(filePath string, root ast.Node, source []byte, fmOffset int) []Edge {
	var out []Edge
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		l, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		// Reference-style link: emit one EdgeRefLink, target is in
		// the same file under the matched label.
		if l.Reference != nil {
			refLabel := string(util.ToLinkReference(l.Reference.Value))
			line, col := nodePosition(source, l)
			out = append(out, Edge{
				SourceFile:  filePath,
				SourceLine:  line + fmOffset,
				SourceCol:   col,
				TargetLabel: refLabel,
				Kind:        EdgeRefLink,
			})
			return ast.WalkContinue, nil
		}

		dest := string(l.Destination)
		t, ok := parseLinkTarget(dest)
		if !ok {
			return ast.WalkContinue, nil
		}
		line, col := nodePosition(source, l)
		// parseLinkTarget guarantees t.LocalAnchor || t.Path != "":
		// it returns false otherwise. So the LocalAnchor branch and
		// the file-link branch together cover every parsed target.
		if t.LocalAnchor {
			out = append(out, Edge{
				SourceFile:   filePath,
				SourceLine:   line + fmOffset,
				SourceCol:    col,
				TargetAnchor: mdtext.Slugify(decodeAnchor(t.Anchor)),
				Kind:         EdgeAnchorLink,
			})
		} else {
			tgt := resolveRelTarget(filePath, t.Path)
			if tgt == "" {
				// Absolute or escapes-the-root paths cannot point
				// at anything inside the workspace. Emitting an
				// edge with empty TargetFile would be ambiguous —
				// IncomingEdges treats `""` as "same file as
				// source", so the link would be misattributed as a
				// self-reference. Drop it instead.
				return ast.WalkContinue, nil
			}
			out = append(out, Edge{
				SourceFile:   filePath,
				SourceLine:   line + fmOffset,
				SourceCol:    col,
				TargetFile:   tgt,
				TargetAnchor: mdtext.Slugify(decodeAnchor(t.Anchor)),
				Kind:         EdgeFileLink,
			})
		}
		return ast.WalkContinue, nil
	})
	return out
}

// linkTarget mirrors the parsed shape of a link destination.
type linkTarget struct {
	Path        string
	Anchor      string
	LocalAnchor bool
}

func parseLinkTarget(dest string) (linkTarget, bool) {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.HasPrefix(dest, "//") {
		return linkTarget{}, false
	}
	u, err := url.Parse(dest)
	if err != nil {
		return linkTarget{}, false
	}
	if u.Scheme != "" || u.Host != "" {
		return linkTarget{}, false
	}
	// u.Opaque is non-empty only when there's a scheme; the scheme
	// check above already rejects those cases, so we don't need to
	// fold u.Opaque into u.Path here.
	p := u.Path
	if p == "" && u.Fragment != "" {
		return linkTarget{Anchor: u.Fragment, LocalAnchor: true}, true
	}
	if p == "" {
		return linkTarget{}, false
	}
	return linkTarget{Path: p, Anchor: u.Fragment}, true
}

func decodeAnchor(s string) string {
	if d, err := url.PathUnescape(s); err == nil {
		return d
	}
	return s
}

// ResolveRelTarget joins srcFile's directory with linkPath and
// returns the workspace-relative result. Absolute paths and ones
// that escape the workspace root after normalization return the
// empty string — callers must treat "" as "no in-workspace target"
// rather than as a valid path. Exported so the LSP server's
// directive-arg navigation paths can apply the same escape rules
// as the index's edge collector.
func ResolveRelTarget(srcFile, linkPath string) string {
	return resolveRelTarget(srcFile, linkPath)
}

// resolveRelTarget is the package-internal implementation of
// ResolveRelTarget; see that function's doc. The function is
// strict about its inputs:
//
//   - srcFile must already be workspace-relative (no leading `/`,
//     no drive letter, no UNC `\\` prefix). Callers that hold
//     absolute paths must run them through workspaceRelative
//     first; otherwise a `../../etc/passwd`-style linkPath could
//     escape via path.Join's absolute-path semantics.
//   - linkPath has both `\` and `/` translated to `/` before
//     joining so a Windows-authored `sub\x.md` resolves the same
//     way on Linux. (filepath.ToSlash is OS-dependent and a no-op
//     on POSIX hosts; the rest of the codebase translates
//     explicitly via strings.ReplaceAll, see NormalizePath.)
//   - Both `path.IsAbs` and the cleaned-path escape check fire as
//     belt-and-suspenders: an absolute linkPath, an absolute
//     srcFile, or any combination that produces an absolute
//     cleaned path returns "".
func resolveRelTarget(srcFile, linkPath string) string {
	srcFile = strings.ReplaceAll(srcFile, `\`, `/`)
	linkPath = strings.ReplaceAll(linkPath, `\`, `/`)
	if path.IsAbs(srcFile) || path.IsAbs(linkPath) {
		return ""
	}
	if isDriveOrUNC(srcFile) || isDriveOrUNC(linkPath) {
		return ""
	}
	dir := path.Dir(srcFile)
	cleaned := path.Clean(path.Join(dir, linkPath))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	if path.IsAbs(cleaned) {
		return ""
	}
	return cleaned
}

// isDriveOrUNC reports whether p starts with a Windows drive
// letter (e.g. `C:`) or a UNC prefix (`//server`). Callers use
// this to refuse non-relative inputs even when running on POSIX
// hosts, where filepath.IsAbs wouldn't flag them.
func isDriveOrUNC(p string) bool {
	if len(p) >= 2 && p[1] == ':' {
		c := p[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	return strings.HasPrefix(p, "//")
}

// nodePosition returns the 1-based source line and column of the
// first text segment under n. Falls back to (1, 1) when no text is
// found.
func nodePosition(source []byte, n ast.Node) (int, int) {
	off := -1
	_ = ast.Walk(n, func(cur ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := cur.(*ast.Text); ok {
			if off < 0 || t.Segment.Start < off {
				off = t.Segment.Start
			}
		}
		return ast.WalkContinue, nil
	})
	if off < 0 {
		return 1, 1
	}
	line := lineOfOffset(source, off)
	lineStart := 0
	for i := 0; i < off && i < len(source); i++ {
		if source[i] == '\n' {
			lineStart = i + 1
		}
	}
	return line, off - lineStart + 1
}

// collectDirectiveEdges emits one Edge per `<?include?>`,
// `<?catalog?>`, and `<?build?>` directive whose body specifies a
// concrete target. Catalog edges aggregate to one entry per directive
// (the design's "first pass" decision).
func collectDirectiveEdges(filePath string, root ast.Node, source []byte, fmOffset int) []Edge {
	var out []Edge
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*lint.ProcessingInstruction)
		if !ok {
			continue
		}
		if strings.HasPrefix(pi.Name, "/") {
			continue
		}
		startLine, _ := piLineRange(pi, source, fmOffset)
		params, ok := parsePIParams(pi, source)
		if !ok {
			continue
		}
		// Empty resolveRelTarget means the target is absolute or
		// escapes the workspace root. Recording it would surface as
		// an empty TargetFile, which IncomingEdges treats as
		// "same file as source" and would silently misattribute the
		// reference back to the host file.
		switch pi.Name {
		case "include":
			if file := strings.TrimSpace(params["file"]); file != "" {
				if tgt := resolveRelTarget(filePath, file); tgt != "" {
					out = append(out, Edge{
						SourceFile: filePath,
						SourceLine: startLine,
						SourceCol:  1,
						TargetFile: tgt,
						Kind:       EdgeInclude,
					})
				}
			}
		case "build":
			if src := strings.TrimSpace(params["source"]); src != "" {
				if tgt := resolveRelTarget(filePath, src); tgt != "" {
					out = append(out, Edge{
						SourceFile: filePath,
						SourceLine: startLine,
						SourceCol:  1,
						TargetFile: tgt,
						Kind:       EdgeBuild,
					})
				}
			}
		case "catalog":
			// Catalog targets are globs; the index records one
			// edge with empty TargetFile so call-hierarchy can
			// surface "this file uses a catalog" without exploding
			// every match into a separate entry. callers that want
			// expansion can resolve the glob themselves.
			out = append(out, Edge{
				SourceFile: filePath,
				SourceLine: startLine,
				SourceCol:  1,
				Kind:       EdgeCatalog,
			})
		}
	}
	return out
}

// parsePIParams converts a PI block's YAML body into a flat string
// map. Single-line PIs (no body) yield an empty map and ok=true.
func parsePIParams(pi *lint.ProcessingInstruction, source []byte) (map[string]string, bool) {
	body := extractPIBody(pi, source)
	mp := MarkerPairLike{
		StartLine: lineOfOffset(source, pi.Lines().At(0).Start),
		YAMLBody:  body,
	}
	return parseYAMLBody(mp)
}

// MarkerPairLike mirrors gensection.MarkerPair without the dependency,
// since we only use the YAMLBody field.
type MarkerPairLike struct {
	StartLine int
	YAMLBody  string
}

func parseYAMLBody(mp MarkerPairLike) (map[string]string, bool) {
	mpReal := gensection.MarkerPair{StartLine: mp.StartLine, YAMLBody: mp.YAMLBody}
	rawMap, diags := gensection.ParseYAMLBody("", mpReal, "", "")
	if len(diags) > 0 {
		return nil, false
	}
	gensection.ExtractColumnsRaw(rawMap)
	params, diags := gensection.ValidateStringParams("", mp.StartLine, rawMap, "", "")
	if len(diags) > 0 {
		return nil, false
	}
	return params, true
}

func extractPIBody(pi *lint.ProcessingInstruction, source []byte) string {
	lines := pi.Lines()
	if lines.Len() <= 1 {
		return ""
	}
	var b strings.Builder
	for i := 1; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}
