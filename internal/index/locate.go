package index

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// TokenTag classifies the shape of source token under the cursor.
// LSP definition / implementation / references handlers branch on
// this to decide what to resolve.
type TokenTag int

const (
	// TokenNone is the catch-all "cursor is on plain prose" tag.
	TokenNone TokenTag = iota
	// TokenHeading is the cursor on a heading line.
	TokenHeading
	// TokenAnchorLink is the cursor inside an `[…](#anchor)` link.
	TokenAnchorLink
	// TokenFileLink is the cursor inside an `[…](./other.md…)` link.
	TokenFileLink
	// TokenRefUse is the cursor inside an `[…][label]` reference-style link.
	TokenRefUse
	// TokenRefDef is the cursor inside a `[label]: url` definition.
	TokenRefDef
	// TokenDirectiveArg is the cursor on a directive argument value
	// (e.g. the `file:` value inside `<?include?>`).
	TokenDirectiveArg
	// TokenFrontMatterKey is the cursor on a top-level FM key.
	TokenFrontMatterKey
	// TokenFrontMatterValue is the cursor on a value beside a top-level
	// FM key — currently used only for `kind:` value lookups.
	TokenFrontMatterValue
	// TokenFileTop is the cursor at the first line of the body —
	// the line immediately after any stripped YAML front matter,
	// not necessarily line 1 of the source. The locator strips
	// `---\n…\n---\n` before tagging, so on a file with front
	// matter (line 1, col 1) lands inside the front-matter range
	// and surfaces as TokenFrontMatterKey instead.
	TokenFileTop
)

// Locator walks one parsed file's source and resolves a 1-based
// (line, character) position to a TokenTag plus the relevant payload.
//
// Ranges are computed in mdsmith coordinates (1-based lines and
// columns); LSP-coordinate translation is the LSP layer's job. The
// returned LocateResult is meant to be self-contained: every field
// the caller needs to issue a follow-up Lookup is on the result, so
// the LSP handler does not need to re-parse.
type Locator struct {
	Path string // workspace-relative path
}

// LocateResult is the payload of a successful Locate call.
type LocateResult struct {
	Tag TokenTag

	// Heading: the slug.
	Anchor string
	// Heading: the level.
	Level int
	// Heading text (the displayed name).
	Name string

	// File link target (workspace-relative, from the cursor's host
	// file). Empty for anchor-only or ref-style links.
	TargetFile string
	// File link / anchor link: the target's heading anchor (slug).
	TargetAnchor string

	// Reference link / def label, normalized.
	Label string

	// Directive name when on a TokenDirectiveArg.
	DirectiveName string
	// Directive argument key (e.g. "file" or "source").
	DirectiveArg string
	// Directive argument value (raw, untrimmed).
	DirectiveValue string
	// DirectiveTargetFile is the raw `file:` (for include) or
	// `source:` (for build) value the cursor sits on, copied
	// verbatim from the directive body. It is *not* resolved
	// against the host file's directory — the LSP layer pipes it
	// through index.ResolveRelTarget, which applies the same
	// escape-the-root rejection rules as the edge collector.
	DirectiveTargetFile string

	// Front-matter key (when on TokenFrontMatterKey/Value).
	FrontMatterKey string
	// Front-matter value when on TokenFrontMatterValue.
	FrontMatterValue string
}

// Locate returns the token tag at (line, col) in source. line and
// col are 1-based; col counts UTF-8 bytes (consistent with the rest
// of the index). When no specific tag fits, TokenNone is returned.
//
// The function operates on whatever bytes the caller hands in, so
// the LSP layer can call it on the live editor buffer without first
// landing the change in the index.
func (l Locator) Locate(source []byte, line, col int) LocateResult {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	fmBytes, body := lint.StripFrontMatter(source)
	fmOffset := 0
	if len(fmBytes) > 0 {
		fmOffset = bytes.Count(fmBytes, []byte{'\n'})
	}

	// Front-matter scope: cursor sits inside the prefix.
	if line <= fmOffset {
		return locateInFrontMatter(fmBytes, line, col)
	}

	// Body scope: shift line back into body coordinates.
	bodyLine := line - fmOffset
	if bodyLine == 1 && col == 1 {
		return LocateResult{Tag: TokenFileTop}
	}

	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	lines := bytes.Split(body, []byte("\n"))

	// Walk each AST node looking for one whose range covers the cursor.
	if res, ok := locateInAST(l.Path, root, body, lines, bodyLine, col); ok {
		return res
	}

	// Heading line check (cursor on a heading not yet matched as a link).
	if h := headingOnLine(root, body, bodyLine); h != nil {
		anchor, level, name := headingInfo(h, body, root)
		return LocateResult{
			Tag:    TokenHeading,
			Anchor: anchor,
			Level:  level,
			Name:   name,
		}
	}

	// Reference definition on this line?
	if r, ok := refDefOnLine(body, lines, bodyLine, col); ok {
		return r
	}

	return LocateResult{Tag: TokenNone}
}

// headingInfo returns the slug, level, and plain text of a heading
// node, walking the root once to account for duplicate-slug
// disambiguation.
func headingInfo(target *ast.Heading, source []byte, root ast.Node) (string, int, string) {
	usedAnchors := map[string]bool{}
	slugCounts := map[string]int{}
	var anchor string
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		txt := mdtext.ExtractPlainText(h, source)
		slug := mdtext.Slugify(txt)
		a := slug
		if a != "" && usedAnchors[a] {
			c := slugCounts[slug]
			for {
				c++
				a = fmt.Sprintf("%s-%d", slug, c)
				if !usedAnchors[a] {
					break
				}
			}
			slugCounts[slug] = c
		}
		if a != "" {
			usedAnchors[a] = true
		}
		if h == target {
			anchor = a
		}
		return ast.WalkContinue, nil
	})
	name := mdtext.ExtractPlainText(target, source)
	return anchor, target.Level, name
}

// locateInAST walks AST nodes whose source range covers the cursor.
// Returns a LocateResult and true on first match. srcPath is the
// workspace-relative path of the document being inspected; it's
// used to resolve relative file links into workspace-relative form.
func locateInAST(srcPath string, root ast.Node, source []byte, lines [][]byte, line, col int) (LocateResult, bool) {
	cursorOff := offsetAt(lines, line, col)
	var found *ast.Link
	var foundPI *lint.ProcessingInstruction
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Inline ast.Link: check if cursor is within its range.
		if l, ok := n.(*ast.Link); ok {
			if linkContainsOffset(source, l, cursorOff) {
				found = l
				return ast.WalkStop, nil
			}
		}
		// Block-level processing-instruction.
		if pi, ok := n.(*lint.ProcessingInstruction); ok {
			if piContainsLine(source, pi, line) {
				foundPI = pi
			}
		}
		return ast.WalkContinue, nil
	})
	if found != nil {
		return linkToLocate(srcPath, found, source), true
	}
	if foundPI != nil {
		return piToLocate(foundPI, source, lines, line, col), true
	}
	return LocateResult{}, false
}

func linkContainsOffset(source []byte, l *ast.Link, off int) bool {
	// Approximate the range from the link's child text segments.
	// For ast.Link, the children carry the visible text; we widen
	// the range to cover the surrounding `[...](...)` syntax by
	// scanning the source line.
	startOff := -1
	endOff := -1
	_ = ast.Walk(l, func(cur ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := cur.(*ast.Text); ok {
			if startOff < 0 || t.Segment.Start < startOff {
				startOff = t.Segment.Start
			}
			if t.Segment.Stop > endOff {
				endOff = t.Segment.Stop
			}
		}
		return ast.WalkContinue, nil
	})
	if startOff < 0 {
		return false
	}
	// Widen left to '[' and right to the matching closing delimiter
	// on the same line. Reference-style links close with `]` after
	// the label (`[text][label]`); inline links close with `)`
	// after the destination (`[text](dest)`). Bounding only on
	// end-of-line was too loose: it tagged plain prose typed after
	// a link as still being "inside" the link, so cursor → token
	// resolution fired definition / references for unrelated
	// positions.
	open := bytes.LastIndexByte(source[:startOff], '[')
	if open < 0 || open < startOff-200 {
		open = startOff
	}
	close := linkCloseOffset(source, l, endOff)
	if close < 0 {
		// Couldn't find a closing delimiter (malformed link); fall
		// back to the text segment so we don't claim coverage of
		// the entire source line.
		close = endOff
	}
	if open <= off && off <= close {
		return true
	}
	return false
}

// linkCloseOffset returns the byte offset of the closing delimiter
// for a link whose display text ends at `after` (the byte offset
// just past the last text character — i.e., on the closing `]` of
// the text portion). Goldmark emits four link shapes:
//
//   - `[text](dest)` — inline. The cursor-in-link range extends
//     through the matching `)` (parens may nest, e.g. URL
//     parameters with parens inside).
//   - `[text][label]` — full reference. Range extends through the
//     second `]`.
//   - `[text][]` — collapsed reference. Range extends through the
//     second `]` (which is one byte after the empty `[]`).
//   - `[label]` — shortcut reference. Range extends through the
//     single `]` at `after`.
//
// Reference shape is detected via `l.Reference.Type`; inline shape
// is detected by the `(` that follows the text-closing `]`. Stops
// at newline so multi-line prose past the link can't be
// misattributed back to it.
func linkCloseOffset(source []byte, l *ast.Link, after int) int {
	if l != nil && l.Reference != nil {
		switch l.Reference.Type {
		case ast.ReferenceLinkShortcut:
			// `[label]` — close at the `]` at or after `after`.
			return scanForByte(source, after, ']')
		default:
			// Full / collapsed: skip the text-closing `]`, then
			// the opening `[` of the label/empty-pair, then close
			// at the next `]`.
			i := scanForByte(source, after, ']')
			if i < 0 {
				return -1
			}
			i = scanForByte(source, i+1, ']')
			return i
		}
	}
	// Inline `[text](dest)`: skip past the `]` to find `(`, then
	// match the closing `)` while accounting for nested parens.
	i := after
	for i < len(source) && source[i] == ']' {
		i++
	}
	if i >= len(source) || source[i] != '(' {
		// Not an inline link shape; treat the next `]` as close.
		return scanForByte(source, after, ']')
	}
	depth := 0
	for ; i < len(source); i++ {
		switch source[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		case '\n':
			return -1
		}
	}
	return -1
}

// scanForByte returns the offset of the next `target` byte at or
// after `from`, or -1 when a newline is hit first.
func scanForByte(source []byte, from int, target byte) int {
	for i := from; i < len(source); i++ {
		switch source[i] {
		case target:
			return i
		case '\n':
			return -1
		}
	}
	return -1
}

func piContainsLine(source []byte, pi *lint.ProcessingInstruction, line int) bool {
	startSeg := pi.Lines().At(0)
	startLine := lineOfOffset(source, startSeg.Start)
	endLine := startLine
	if pi.HasClosure() && pi.ClosureLine.Start > startSeg.Start {
		endLine = lineOfOffset(source, pi.ClosureLine.Start)
	}
	return line >= startLine && line <= endLine
}

// linkToLocate produces a LocateResult for a link node. srcPath is
// the workspace-relative path of the document — relative file links
// are resolved against srcPath's directory so the returned
// TargetFile is itself workspace-relative. Without that resolution,
// `[x](./b.md)` from `docs/a.md` would surface as `b.md` at the
// workspace root and definition would jump to the wrong file (or
// nowhere); paths that escape the workspace via `..` resolve to "".
func linkToLocate(srcPath string, l *ast.Link, source []byte) LocateResult {
	if l.Reference != nil {
		return LocateResult{
			Tag:   TokenRefUse,
			Label: string(util.ToLinkReference(l.Reference.Value)),
		}
	}
	dest := string(l.Destination)
	t, ok := linkgraph.ParseTarget(dest)
	if !ok {
		return LocateResult{Tag: TokenNone}
	}
	if t.LocalAnchor {
		return LocateResult{
			Tag:          TokenAnchorLink,
			TargetAnchor: mdtext.Slugify(linkgraph.DecodeAnchor(t.Anchor)),
		}
	}
	return LocateResult{
		Tag:          TokenFileLink,
		TargetFile:   linkgraph.ResolveRelTarget(srcPath, t.Path),
		TargetAnchor: mdtext.Slugify(linkgraph.DecodeAnchor(t.Anchor)),
	}
}

// piToLocate inspects a directive PI and returns the relevant payload
// for the cursor's specific argument line. The directive name surfaces
// as DirectiveName; the argument key/value the cursor sits on lands in
// DirectiveArg/DirectiveValue.
func piToLocate(pi *lint.ProcessingInstruction, source []byte, lines [][]byte, line, col int) LocateResult {
	res := LocateResult{
		Tag:           TokenDirectiveArg,
		DirectiveName: pi.Name,
	}
	// Find the line the cursor is on within the PI body.
	if line > len(lines) {
		return res
	}
	cursorLine := string(lines[line-1])
	// Match `key: value` where value may be quoted.
	m := piArgRE.FindStringSubmatch(cursorLine)
	if len(m) >= 3 {
		res.DirectiveArg = m[1]
		res.DirectiveValue = strings.Trim(strings.TrimSpace(m[2]), `"'`)
	}
	if (pi.Name == "include" && res.DirectiveArg == "file") ||
		(pi.Name == "build" && res.DirectiveArg == "source") {
		res.DirectiveTargetFile = res.DirectiveValue
	}
	return res
}

// piArgRE matches a YAML-ish `key: value` line. Values keep their
// quote characters so the caller can decide whether to strip them.
var piArgRE = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:\s*(.*?)\s*$`)

// headingOnLine returns the heading whose first source line equals
// line, or nil.
func headingOnLine(root ast.Node, source []byte, line int) *ast.Heading {
	var found *ast.Heading
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		startLine := lineOfOffset(source, h.Lines().At(0).Start)
		if startLine == line {
			found = h
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// refDefOnLine returns a TokenRefDef LocateResult when (line, col) sits
// on a `[label]: url` line. The label is filled in.
func refDefOnLine(body []byte, lines [][]byte, line, col int) (LocateResult, bool) {
	if line < 1 || line > len(lines) {
		return LocateResult{}, false
	}
	src := lines[line-1]
	m := refDefRE.FindSubmatchIndex(src)
	if len(m) < 4 {
		return LocateResult{}, false
	}
	label := string(src[m[2]:m[3]])
	return LocateResult{
		Tag:   TokenRefDef,
		Label: string(util.ToLinkReference([]byte(label))),
	}, true
}

// locateInFrontMatter resolves a cursor within the YAML front matter
// to a key or value token. Three line shapes are recognized:
//
//   - `key: value` scalar — column ≤ colon yields TokenFrontMatterKey,
//     past the colon yields TokenFrontMatterValue.
//   - `key:` followed by a block list — the cursor lands on the key.
//   - `  - item` — list-item line under the most recent key. Returns
//     TokenFrontMatterValue with FrontMatterKey set to the parent
//     key and FrontMatterValue set to the trimmed item, so callers
//     can resolve `kinds:` block-list assignments the same way
//     they handle inline `kind: value`.
//
// The trailing-key form is the only case the previous regex-only
// approach mishandled — Markdown `kinds:` lists are the canonical
// way to assign kinds, so missing them broke navigation on every
// real workspace.
func locateInFrontMatter(fm []byte, line, col int) LocateResult {
	if line < 1 {
		return LocateResult{Tag: TokenNone}
	}
	lines := bytes.Split(fm, []byte("\n"))
	if line-1 >= len(lines) {
		return LocateResult{Tag: TokenNone}
	}
	row := string(lines[line-1])
	if item, ok := frontMatterListItem(row); ok {
		parent := frontMatterParentKey(lines, line-1)
		return LocateResult{
			Tag:              TokenFrontMatterValue,
			FrontMatterKey:   parent,
			FrontMatterValue: item,
		}
	}
	colonIdx := strings.IndexByte(row, ':')
	if colonIdx < 0 {
		return LocateResult{Tag: TokenNone}
	}
	key := strings.TrimSpace(row[:colonIdx])
	value := strings.TrimSpace(strings.Trim(row[colonIdx+1:], `"' `))
	if col-1 <= colonIdx {
		return LocateResult{
			Tag:            TokenFrontMatterKey,
			FrontMatterKey: key,
		}
	}
	return LocateResult{
		Tag:              TokenFrontMatterValue,
		FrontMatterKey:   key,
		FrontMatterValue: value,
	}
}

// frontMatterListItem returns the trimmed item value when row is a
// YAML block-list line ("  - foo" or "- foo"). Inline `[a, b]` and
// quoted forms are intentionally not handled here — those parse as
// scalars on the parent line.
func frontMatterListItem(row string) (string, bool) {
	trimmed := strings.TrimLeft(row, " \t")
	if !strings.HasPrefix(trimmed, "- ") && trimmed != "-" {
		return "", false
	}
	if trimmed == "-" {
		return "", true
	}
	return strings.Trim(strings.TrimPrefix(trimmed, "- "), `"' `), true
}

// frontMatterParentKey scans backward from idx for the most recent
// line that introduces a key (`name:` form). Lines deeper than the
// item's indent (further to the right) cannot be the parent, but
// the indent calculation here is intentionally lenient: any prior
// `key:` line is treated as a candidate, since the canonical
// `kinds:\n  - foo` form has the parent column 0 and child column 2.
func frontMatterParentKey(lines [][]byte, idx int) string {
	for i := idx - 1; i >= 0; i-- {
		row := string(lines[i])
		trim := strings.TrimSpace(row)
		if trim == "" {
			continue
		}
		// Skip another list item — not a key.
		if strings.HasPrefix(strings.TrimLeft(row, " \t"), "- ") {
			continue
		}
		if c := strings.IndexByte(row, ':'); c >= 0 {
			return strings.TrimSpace(row[:c])
		}
	}
	return ""
}

// offsetAt converts a 1-based (line, col) position to a byte offset
// in the buffer represented by lines.
func offsetAt(lines [][]byte, line, col int) int {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	off := 0
	for i := 0; i < line-1 && i < len(lines); i++ {
		off += len(lines[i]) + 1
	}
	if line-1 < len(lines) {
		c := col - 1
		if c > len(lines[line-1]) {
			c = len(lines[line-1])
		}
		off += c
	}
	return off
}

// SafeURLPathEscape applies net/url.PathEscape to s. PathEscape
// percent-encodes every byte that would be unsafe in a URL path
// segment — spaces, slashes, `%`, and a long list of reserved /
// non-ASCII characters — so the result is guaranteed safe to
// drop into a `file://` URI fragment without further encoding.
// Despite the name, the function is not limited to "%" sequences;
// callers that need a more permissive encoding should reach for
// QueryEscape directly.
func SafeURLPathEscape(s string) string {
	return url.PathEscape(s)
}
