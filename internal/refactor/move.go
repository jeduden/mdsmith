package refactor

import (
	"bytes"
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// ErrTraversalPath is returned when a move source or destination
// escapes the workspace (an absolute path or one that resolves outside
// the root). Nothing is moved or written.
var ErrTraversalPath = errors.New("path escapes the workspace")

// ErrSameFile is returned when a move's source and destination
// normalize to the same workspace-relative path.
var ErrSameFile = errors.New("source and destination are the same file")

// DestinationExistsError reports that a move's destination already
// exists. The move aborts with no edit written, so an existing file is
// never clobbered.
type DestinationExistsError struct{ Dst string }

func (e DestinationExistsError) Error() string {
	return "destination already exists: " + e.Dst
}

// SourceNotFoundError reports that a move's source file is not readable
// in the workspace.
type SourceNotFoundError struct{ Src string }

func (e SourceNotFoundError) Error() string {
	return "source file not found: " + e.Src
}

// Move computes a Plan that relocates the workspace file src to dst and
// rewrites every reference so no link breaks in either direction. The
// returned Plan carries a FileOp{From: src, To: dst} the host executes
// after applying the edits (a git mv when tracked, a plain rename
// otherwise); the engine never touches the filesystem.
//
// The Plan rewrites, keyed per output target:
//
//   - incoming destinations — every inline link, image, and
//     reference definition in another file whose destination names
//     src, its path token rewritten to name dst, any `?query` and
//     `#fragment` kept;
//   - wikilink stems — `[[old-stem]]` → `[[new-stem]]`, but only when
//     the basename stem changes; a move that keeps the basename leaves
//     wikilinks alone because a stem still resolves (a documented
//     asymmetry with path links). Only a Markdown src with a non-empty
//     stem is a stem target, and a dst no wikilink can name — no
//     extension, an empty stem, a `#`, `|`, `[`, `]`, CR, or newline in
//     the name, or a name that starts or ends with a space — gets no
//     rewrite;
//   - outbound destinations inside src, when it has a Markdown
//     extension or the workspace lists it (an `.mdx` file that
//     `files:` matches) — every `[t](path)`, `![a](path)` and
//     `[label]: path` recomputed so it still resolves from dst's
//     directory. Another file, such as an image, keeps its bytes.
//
// Every destination is found in the parsed document (see destLocator),
// so each one is rewritten exactly once, at its own bytes: an empty
// label, a label or destination split across rows, and a `](`-shaped
// string inside a label or code span are all handled. A
// percent-escaped destination is decoded before it is compared, and
// the new path is escaped the way the author escaped the old one (see
// encodeLike).
//
// Spelling is preserved: an explicit `./x` keeps its prefix. Absolute
// URLs, mailto, root-anchored `/x`, and any other out-of-workspace
// path are never touched — they do not resolve to a workspace file, so
// a move has nothing to rewrite.
//
// A traversal path returns ErrTraversalPath; an equal src/dst returns
// ErrSameFile; a missing source returns SourceNotFoundError; an
// existing destination returns DestinationExistsError. Each aborts with
// a zero Plan and no edit.
//
// `<?include?>`, `<?build?>`, and `<?catalog?>` directive paths are
// not yet recomputed, so a cross-directory move can leave them stale —
// a tracked follow-up.
func Move(ws Workspace, src, dst string) (Plan, error) {
	src = index.NormalizePath(src)
	dst = index.NormalizePath(dst)
	if !workspaceRelative(src) || !workspaceRelative(dst) {
		return Plan{}, ErrTraversalPath
	}
	if src == dst {
		return Plan{}, ErrSameFile
	}
	srcKey, srcSource, ok := ws.Resolve(src)
	if !ok {
		return Plan{}, SourceNotFoundError{Src: src}
	}
	if _, _, exists := ws.Resolve(dst); exists {
		return Plan{}, DestinationExistsError{Dst: dst}
	}

	changes := map[string][]Edit{}
	p := lint.NewParser()
	r := &destResolver{ws: ws, src: src}
	appendReferrerEdits(changes, ws, p, r, src, dst)
	appendWikilinkStemEdits(changes, ws, src, dst)
	if mdpath.HasMarkdownExt(path.Ext(src)) || r.listed(src) {
		appendOutboundEdits(changes, p, r, srcKey, src, dst, srcSource)
	}
	stableSortEdits(changes)
	return Plan{Edits: changes, FileOp: &FileOp{From: src, To: dst}}, nil
}

// workspaceRelative reports whether p is a safe workspace-relative path
// — not absolute, not a `..` traversal. p is assumed already
// NormalizePath-cleaned (forward slashes, no leading `./`).
func workspaceRelative(p string) bool {
	if p == "" {
		return false
	}
	t := filepath.ToSlash(p)
	if path.IsAbs(t) {
		return false
	}
	cleaned := path.Clean(t)
	return cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

// relFrom returns the clean forward-slash path from fromDir to target,
// both workspace-relative; filepath.Rel spells the path to the root
// `.` from `docs` as `../.`. It falls back to target on the rare error
// path (paths on different volumes), which cannot happen for two
// workspace-relative inputs.
func relFrom(fromDir, target string) string {
	r, err := filepath.Rel(fromDir, target)
	if err != nil {
		return target
	}
	return path.Clean(filepath.ToSlash(r))
}

var (
	linkMark   = []byte("](")
	refDefMark = []byte("]:")
)

// appendReferrerEdits repoints every destination in another workspace
// file that names src — inline links, images, and reference
// definitions — so it names dst. A self-reference inside src is left
// to the outbound pass, so no token is edited twice.
//
// Every file is read, but only one mayName admits is parsed. The
// index is not consulted: it records no edge for an image or a
// ref-def, and it reads a literal `what?.md` as `what`.
func appendReferrerEdits(
	changes map[string][]Edit, ws Workspace, p parser.Parser, r *destResolver, src, dst string,
) {
	base := []byte(path.Base(src))
	for _, rel := range ws.Files() {
		rel = index.NormalizePath(rel)
		if rel == src {
			continue
		}
		key, source, ok := ws.Resolve(rel)
		if !ok || !mayName(source, base) {
			continue
		}
		for _, d := range locateDests(p, rel, source) {
			ref, ok := r.target(rel, d.dest)
			if !ok || ref.target != src {
				continue
			}
			if edit, ok := destEdit(d, ref, rel, dst); ok {
				changes[key] = append(changes[key], edit)
			}
		}
	}
}

// mayName reports whether source may hold a destination that names a
// file with the base name base. It needs a `](` or `]:` to open one,
// and base written out or a `%` that may escape it: a destination's
// path, once decoded and cleaned, ends in the base name it names, and
// cleaning only drops path segments.
func mayName(source, base []byte) bool {
	if !bytes.Contains(source, linkMark) && !bytes.Contains(source, refDefMark) {
		return false
	}
	return bytes.Contains(source, base) || bytes.IndexByte(source, '%') >= 0
}

// appendOutboundEdits recomputes every relative inline link, image and
// reference-definition destination inside the moved file so it still
// resolves from dst's directory. Edits key under the moved file's own key: the host applies
// them before the file relocates.
func appendOutboundEdits(
	changes map[string][]Edit, p parser.Parser, r *destResolver, srcKey, src, dst string, source []byte,
) {
	for _, d := range locateDests(p, src, source) {
		if edit, ok := outboundEdit(r, d, src, dst); ok {
			changes[srcKey] = append(changes[srcKey], edit)
		}
	}
}

// outboundEdit re-spells one destination of the moved file so it still
// resolves from dst's directory. It reports ok=false for a same-file
// anchor, an external or out-of-workspace destination, and one that
// still names its target from dst's directory.
func outboundEdit(r *destResolver, d inlineDest, src, dst string) (Edit, bool) {
	ref, ok := r.target(src, d.dest)
	if !ok {
		return Edit{}, false
	}
	// A path link inside src that points at src itself must keep
	// pointing at the file after it relocates, so recompute against
	// dst — otherwise the token would be rewritten to address the
	// old (now vacated) location.
	tgt := ref.target
	if tgt == src {
		tgt = dst
	}
	// The reference lives in the moved file, so its new spelling is
	// computed as if from dst's directory.
	return destEdit(d, ref, dst, tgt)
}

// destEdit rewrites the path token of d (read as ref) so that, from
// spellFrom's directory, it names target. ok is false when the token
// already names target from there, so a spelling such as
// `sub/../b.md` is kept while it still resolves. An explicit `./x`
// keeps its prefix unless the new path starts with `.` or `..`, which
// already reads as relative; everything else is bare-relative. A new
// path whose first segment holds a `:` gets a `./` prefix too: a bare
// `a:b.md` reads as the URL scheme `a:`. A link to a directory keeps
// its trailing `/`.
func destEdit(d inlineDest, ref destRef, spellFrom, target string) (Edit, bool) {
	if linkgraph.ResolveRelTarget(spellFrom, ref.path) == target {
		return Edit{}, false
	}
	newPath := relFrom(path.Dir(spellFrom), target)
	first, _, _ := strings.Cut(newPath, "/")
	if strings.HasPrefix(ref.path, "./") && first != "." && first != ".." ||
		strings.IndexByte(first, ':') >= 0 {
		newPath = "./" + newPath
	}
	if ref.dir {
		newPath += "/"
	}
	pe := d.ps + ref.tokLen
	return Edit{
		Range: Range{
			Start: Position{Line: d.line, Character: mdtext.UTF16FromByteOffset(d.row, d.ps)},
			End:   Position{Line: d.line, Character: mdtext.UTF16FromByteOffset(d.row, pe)},
		},
		NewText: encodeLike(newPath, string(d.row[d.ps:pe]), d.angle),
	}, true
}

// destRef is one destination read the way the index reads it.
type destRef struct {
	target string // the workspace file the destination names
	path   string // the path token, percent-decoded
	tokLen int    // byte length of the path token as written
	dir    bool   // path ends in `/` and target is no file: a directory
}

// destResolver reads destinations for a move of src. It lists the
// workspace's files only when a literal `?` needs them (see target).
type destResolver struct {
	ws    Workspace
	src   string
	files map[string]bool
}

// target reads dest, written in refFile, the way the index does:
// percent-escapes are decoded, and a `?query` or `#fragment` is not
// part of the path. ok is false for an external, anchor-only, or
// out-of-workspace destination.
//
// A literal `?` is where a URL and a file name disagree: `what?.md` is
// the file `what` with the query `.md` to a browser and to the index,
// but a file named `what?.md` may exist. When the query-stripped path
// names no workspace file and the whole path does, the whole path is
// the target, so a move never truncates a real file name. The rewrite
// then escapes that `?` as `%3F`, which both readings agree on.
//
// ok is also false when the path token holds a backslash escape or an
// entity (see markupEscaped): a renderer reads `a\_b.md` as `a_b.md`
// and `a&amp;b.md` as `a&b.md`, which neither the index nor a rewrite
// decodes, so the token is left as written. A `\` just before the `#`
// or `?` that ends the path is the one escape read: it only escapes
// that byte, so the path token stops before it.
func (r *destResolver) target(refFile string, dest []byte) (destRef, bool) {
	t, ok := linkgraph.ParseTargetBytes(dest)
	if !ok || t.LocalAnchor {
		return destRef{}, false
	}
	tokLen := len(dest)
	if h := bytes.IndexByte(dest, '#'); h >= 0 {
		tokLen = h
	}
	tgt, p := linkgraph.ResolveRelTarget(refFile, t.Path), t.Path
	if q := bytes.IndexByte(dest[:tokLen], '?'); q >= 0 {
		lit, litTgt := literalTarget(refFile, dest[:tokLen])
		if litTgt != "" && !r.exists(tgt) && r.exists(litTgt) {
			tgt, p = litTgt, lit
		} else {
			tokLen = q
		}
	}
	if tokLen > 1 && tokLen < len(dest) && dest[tokLen-1] == '\\' {
		// `a.md\#x` renders as `a.md#x`: the `\` escapes the `#` or
		// `?` after it and is not part of the path. The rewrite
		// leaves it in place. A lone `\#x` is an anchor, not read.
		tokLen--
		p = strings.TrimSuffix(p, `\`)
		tgt = linkgraph.ResolveRelTarget(refFile, p)
	}
	if tgt == "" || markupEscaped(dest[:tokLen], tokLen < len(dest)) {
		return destRef{}, false
	}
	dir := strings.HasSuffix(p, "/") && !r.exists(tgt)
	return destRef{target: tgt, path: p, tokLen: tokLen, dir: dir}, true
}

// markupEscaped reports whether a renderer reads tok, a destination's
// path token, as other bytes. goldmark resolves a backslash before
// ASCII punctuation and every entity in a destination before it writes
// the link out; a `\` before any other byte, as in a Windows-style
// `sub\a.md`, is kept. more is true when a `?` or `#` follows tok: a
// `\` or `&` just before it escapes that byte or opens a reference
// such as `&#35;`.
func markupEscaped(tok []byte, more bool) bool {
	if n := len(tok); more && n > 0 && (tok[n-1] == '\\' || tok[n-1] == '&') {
		return true
	}
	v := util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(tok)))
	return !bytes.Equal(v, tok)
}

// literalTarget decodes pre, a destination's path and query read as
// one path, and resolves it from refFile. Both results are empty when
// pre does not decode.
func literalTarget(refFile string, pre []byte) (lit, target string) {
	lit, err := url.PathUnescape(string(pre))
	if err != nil {
		return "", ""
	}
	return lit, linkgraph.ResolveRelTarget(refFile, lit)
}

// exists reports whether p names src or another file the workspace
// lists.
func (r *destResolver) exists(p string) bool {
	return p == r.src || r.listed(p)
}

// listed reports whether the workspace lists p. It lists the files
// once, on the first call.
func (r *destResolver) listed(p string) bool {
	if r.files == nil {
		r.files = map[string]bool{}
		for _, f := range r.ws.Files() {
			r.files[index.NormalizePath(f)] = true
		}
	}
	return r.files[p]
}

// encodeLike percent-escapes the path p for the destination token
// oldTok it replaces. Two groups of bytes are escaped:
//
//   - bytes that would end the destination or change what it names:
//     `%`, `?`, `#`, `<`, `>`, `&` (it could start an entity a
//     renderer decodes), `\` (it could escape the next byte), `"`,
//     and control bytes, plus a space unless the destination is
//     angle-bracketed (`<my file.md>`), where a space is literal. A
//     bare destination also escapes every `(` and `)` when its
//     literal parens would not pair up, since an unpaired one ends
//     it early;
//   - bytes the author escaped in oldTok, so `my%20file.md` stays
//     escaped and `caf%C3%A9.md` keeps its escaped UTF-8. One escaped
//     non-ASCII byte escapes them all, so no character is split
//     between the two styles. An escaped letter, digit or `-._~` is
//     not carried over.
//
// A `/` is never escaped: it separates the path's segments. The index
// decodes destinations when it resolves them, so the escaped form
// still names the moved file. A path with nothing to escape (the
// common case) is returned unchanged.
func encodeLike(p, oldTok string, angle bool) string {
	esc := escapeSetFor(oldTok, angle)
	if !angle && !parensPair(p, &esc) {
		esc['('], esc[')'] = true, true
	}
	n := 0
	for i := 0; i < len(p); i++ {
		if esc[p[i]] {
			n++
		}
	}
	if n == 0 {
		return p
	}
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(p) + 2*n)
	for i := 0; i < len(p); i++ {
		c := p[i]
		if !esc[c] {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

// escapeSet marks the bytes encodeLike escapes.
type escapeSet [256]bool

// parensPair reports whether the `(` and `)` that p keeps literal under
// esc pair up, each `)` closing an earlier `(`, as a bare CommonMark
// destination requires.
func parensPair(p string, esc *escapeSet) bool {
	depth := 0
	for i := 0; i < len(p); i++ {
		switch c := p[i]; {
		case esc[c]:
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// escapeSetFor builds encodeLike's escape set for a token written like
// oldTok. A `%` not followed by two hex digits is not an escape and
// adds nothing, and neither does an escaped unreserved byte: one `%2E`
// must not escape every `.` in the new path.
func escapeSetFor(oldTok string, angle bool) escapeSet {
	var s escapeSet
	for c := 0; c < 0x20; c++ {
		s[c] = true
	}
	s[0x7f] = true
	for _, c := range []byte("%?#<>&\\\"") {
		s[c] = true
	}
	s[' '] = !angle
	for i := 0; i+2 < len(oldTok); i++ {
		if oldTok[i] != '%' {
			continue
		}
		hi, okHi := unhex(oldTok[i+1])
		lo, okLo := unhex(oldTok[i+2])
		if !okHi || !okLo {
			continue
		}
		c := hi<<4 | lo
		if !unreserved(c) {
			s[c] = true
		}
		if c >= 0x80 {
			for h := 0x80; h < 0x100; h++ {
				s[h] = true
			}
		}
		i += 2
	}
	s['/'] = false
	return s
}

// unreserved reports whether c is a letter, a digit, or one of `-._~`,
// the bytes a URL never needs to escape.
func unreserved(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		strings.IndexByte("-._~", c) >= 0
}

// unhex returns the value of the hex digit c, in either case. ok is
// false when c is not a hex digit. Only a letter is case-folded: a
// control byte folded with 0x20 would read as a digit.
func unhex(c byte) (v byte, ok bool) {
	if c >= '0' && c <= '9' {
		return c - '0', true
	}
	if l := c | 0x20; l >= 'a' && l <= 'f' {
		return l - 'a' + 10, true
	}
	return 0, false
}

// inlineDest is one destination the locator found: the bytes goldmark
// parsed, and where they sit in the file.
type inlineDest struct {
	dest  []byte // the parsed destination, without any `<` `>`
	row   []byte // the file row holding it
	line  int    // 0-based file row index
	ps    int    // byte offset of dest within row
	angle bool   // written as `<dest>`
}

// locateDests parses a file's source and returns every inline link,
// image and reference-definition destination in it, in document order.
func locateDests(p parser.Parser, file string, source []byte) []inlineDest {
	body, fmOffset := bodyAndFMOffset(source)
	root := p.Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	// The locator reads only Source (for offset-to-row mapping) and the
	// AST; file rows come from fileLines, shifted by fmOffset.
	lf := &lint.File{Path: file, Source: body, AST: root}
	loc := destLocator{lf: lf, fileLines: splitLines(source), fmOffset: fmOffset}
	_ = ast.Walk(root, loc.visit)
	return loc.dests
}

// destLocator finds where each destination sits in a parsed body.
// goldmark records a destination's bytes and where a link or image
// opens (its `[` or `!`), but not where the destination sits. So the
// locator walks the AST with a cursor. Entering a link or image moves
// the cursor to its opening byte. Each text, code-span, raw-HTML and
// autolink segment in its label moves it forward, and so does each
// nested destination and its title; the cursor never moves back. On
// leaving the node, the first `](` at or after the cursor closes that
// node's own label, on whichever row that is. A `](` in a code span,
// an HTML comment, an earlier row, or a nested node's label, title or
// destination is never reached.
// The destination must follow after the spaces, tabs and single line
// ending CommonMark allows, and must match goldmark's bytes exactly;
// anything else is skipped, never guessed at.
//
// A reference definition's destination follows the first `]:` after
// its `[` in the same way.
type destLocator struct {
	lf        *lint.File
	fileLines [][]byte
	fmOffset  int
	cursor    int
	dests     []inlineDest
}

// visit is the ast.Walk callback that drives the cursor.
func (d *destLocator) visit(n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch t := n.(type) {
	case *ast.Text:
		d.advance(entering, t.Segment.Stop)
	case *ast.RawHTML:
		if k := t.Segments.Len(); k > 0 {
			d.advance(entering, t.Segments.At(k-1).Stop)
		}
	case *ast.AutoLink:
		// The label sits between the `<` at Pos() and the closing `>`.
		d.advance(entering, t.Pos()+len(t.Label(d.lf.Source))+2)
	case *ast.Link:
		d.linkNode(entering, t.Pos(), t.Destination, t.Reference == nil)
	case *ast.Image:
		d.linkNode(entering, t.Pos(), t.Destination, t.Reference == nil)
	case *ast.LinkReferenceDefinition:
		// A `^` label is a footnote definition: its text is not a
		// destination, though the parser stores it as one.
		if entering && (len(t.Label) == 0 || t.Label[0] != '^') {
			d.locateRefDef(t)
		}
	}
	return ast.WalkContinue, nil
}

// advance moves the cursor forward to stop when entering a node whose
// bytes end there. It never moves the cursor back: the walk visits
// inline nodes in source order, and an out-of-order stop must not bring
// back a `](` the cursor already passed.
func (d *destLocator) advance(entering bool, stop int) {
	if entering && stop > d.cursor {
		d.cursor = stop
	}
}

// linkNode handles a link or image that opens at pos. Entering it
// moves the cursor to pos. Leaving an inline one locates its
// destination; a reference-style `[a][ref]` has none in the text.
func (d *destLocator) linkNode(entering bool, pos int, dest []byte, inline bool) {
	if entering {
		d.advance(true, pos)
		return
	}
	if inline {
		d.locate(dest)
	}
}

// locate records the destination of the inline node just left and
// moves the cursor past it and past any title, so a `](` inside
// either is never taken for an enclosing label's end.
func (d *destLocator) locate(dest []byte) {
	src := d.lf.Source
	open := labelEnd(src, d.cursor, '(')
	if open < 0 {
		return
	}
	d.advance(true, open)
	start, angle, ok := destStart(src, open, dest)
	if !ok {
		return
	}
	end := start + len(dest)
	if angle {
		end++ // the closing `>`
	}
	d.advance(true, titleEnd(src, end))
	d.record(start, dest, angle)
}

// titleEnd returns the offset just past the title that follows a
// destination ending at i, or i when none does. goldmark parsed the
// link, so a `"`, `'` or `(` after the gap opens its title, and the
// first unescaped closer ends it; a title cannot nest its closer.
func titleEnd(src []byte, i int) int {
	j := skipGap(src, i)
	if j >= len(src) {
		return i
	}
	closer := src[j]
	switch closer {
	case '"', '\'':
	case '(':
		closer = ')'
	default:
		return i
	}
	for k := j + 1; k < len(src); k++ {
		switch src[k] {
		case '\\':
			k++
		case closer:
			return k + 1
		}
	}
	return i
}

// locateRefDef records a reference definition's destination. It
// follows the first `]:` after the definition's `[`, which sits on a
// later row when the label spans rows.
func (d *destLocator) locateRefDef(n *ast.LinkReferenceDefinition) {
	src := d.lf.Source
	colon := labelEnd(src, n.Pos(), ':')
	if colon < 0 {
		return
	}
	if start, angle, ok := destStart(src, colon, n.Destination); ok {
		d.record(start, n.Destination, angle)
	}
}

// record stores the destination that starts at body offset start,
// mapped to its file row.
func (d *destLocator) record(start int, dest []byte, angle bool) {
	line := d.lf.LineOfOffset(start) - 1 + d.fmOffset
	d.dests = append(d.dests, inlineDest{
		dest:  dest,
		row:   d.fileLines[line],
		line:  line,
		ps:    d.lf.ColumnOfOffset(start) - 1,
		angle: angle,
	})
}

// labelEnd returns the offset just past the first unescaped `]` at or
// after from that is followed by next (`(` for an inline link, `:` for
// a reference definition), or -1 when there is none.
func labelEnd(src []byte, from int, next byte) int {
	for i := max(from, 0); i+1 < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case ']':
			if src[i+1] == next {
				return i + 2
			}
		}
	}
	return -1
}

// destStart returns the offset where dest begins after open, the byte
// past a label's `(` or `:`, once skipGap has passed the gap before
// it. angle reports a `<dest>` form. ok is false when goldmark's
// destination bytes are not there.
func destStart(src []byte, open int, dest []byte) (start int, angle, ok bool) {
	i := skipGap(src, open)
	if i < len(src) && src[i] == '<' {
		angle = true
		i++
	}
	end := i + len(dest)
	if end > len(src) || !bytes.Equal(src[i:end], dest) ||
		(angle && (end == len(src) || src[end] != '>')) {
		return 0, false, false
	}
	return i, angle, true
}

// skipGap returns the offset past the gap CommonMark allows between a
// link's parts: spaces, tabs and one line ending. The row after a line
// ending may repeat its container's `>` markers and indentation.
func skipGap(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	if i < len(src) && src[i] == '\r' {
		i++
	}
	if i < len(src) && src[i] == '\n' {
		i++
		for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '>') {
			i++
		}
	}
	return i
}

// appendWikilinkStemEdits rewrites `[[old-stem]]` links to the new
// basename stem, but only when the move changes the basename. A move
// that keeps the basename leaves wikilinks alone: a stem still resolves
// to the file at its new path.
func appendWikilinkStemEdits(changes map[string][]Edit, ws Workspace, src, dst string) {
	// Both ends are keyed the way NewWikilinkIndex keys files. Only a
	// Markdown src has a stem key, so moving any other file retargets
	// no `[[stem]]` link. An empty src key (`docs/.md`) matches no edge,
	// since no target spells it, so the edge lookup below returns early.
	// ` guide.md` keys as " guide": a bare `[[guide]]` never reached it,
	// while a folder-prefixed `[[x/ guide]]` did and is rewritten.
	oldStem, ok := linkgraph.FileStemKey(path.Base(src))
	if !ok {
		return
	}
	// A Markdown destination is addressed by stem, so keeping the stem
	// keeps every link resolving. A typed destination (`guide.png`) is
	// addressed by exact name, a different key space, so it always needs
	// the rewrite: comparing its name to oldStem would wrongly skip a
	// move such as docs/guide.png.md → docs/guide.png.
	newStem, dstIsMarkdown := linkgraph.FileStemKey(path.Base(dst))
	if dstIsMarkdown && oldStem == newStem {
		return
	}
	// The rewritten token must parse back as a wikilink that resolves by
	// dst's key. WikilinkReaches checks that against the wikilink
	// grammar, so a name with no extension (`COPYING`), an empty stem,
	// a `#`, `|`, `[`, `]`, CR, or newline, a name that starts or ends
	// with a space, or a drive-letter shape (`C:x.md`) gets no rewrite.
	newSpelling, ok := dstWikilinkSpelling(dst)
	if !ok {
		return
	}
	// A wikilink resolves by basename stem, and the index keys these
	// edges by stem alone — it cannot record which same-stem file a
	// given `[[stem]]` actually points at. When two or more workspace
	// files share oldStem the rewrite is ambiguous: blindly retargeting
	// every `[[oldStem]]` would rewrite links that resolve to a sibling
	// file that is not moving (e.g. `[[ref/Guide]]` when both
	// docs/Guide.md and ref/Guide.md exist and only docs/Guide.md
	// moves). Leave every such wikilink untouched in that case rather
	// than break an unrelated reference — the moved file's own links
	// stay resolvable by the sibling's stem. The source is counted even
	// when ws.Files() omits it (a `files:` glob can exclude a file
	// Resolve still reads): it holds oldStem either way, so one listed
	// sibling already makes the link ambiguous.
	//
	// The destination stem must be unique too. dst does not exist in the
	// workspace yet (Move rejected an existing destination), so any file
	// already carrying newStem is a *different* file: retargeting
	// `[[oldStem]]` to `[[newStem]]` would make the link resolve to that
	// sibling (or become ambiguous) instead of the moved file. Leave the
	// wikilinks alone, mirroring the source-side ambiguity guard.
	// A Markdown destination is addressed by stem; a typed non-Markdown
	// destination (`guide.mdx`) is addressed by exact file name.
	//
	// Most moves have no `[[oldStem]]` link at all, so the edges are
	// fetched first and the scan over every workspace file is skipped.
	edges := ws.IncomingWikilinkEdges(oldStem)
	if len(edges) == 0 {
		return
	}
	newKey := newStem
	if !dstIsMarkdown {
		newKey = linkgraph.FileNameKey(path.Base(dst))
	}
	oldHolders, newHolders := wikilinkKeyHolders(ws.Files(), src, oldStem, newKey, dstIsMarkdown)
	if oldHolders > 1 || newHolders > 0 {
		return
	}
	for _, e := range edges {
		key, source, ok := ws.Resolve(e.SourceFile)
		if !ok {
			continue
		}
		lines := splitLines(source)
		if e.SourceLine < 1 || e.SourceLine > len(lines) {
			continue
		}
		row := lines[e.SourceLine-1]
		start, end, ok := wikilinkStemBytes(row, e.SourceCol-1)
		if !ok {
			continue
		}
		changes[key] = append(changes[key], Edit{
			Range: Range{
				Start: Position{Line: e.SourceLine - 1, Character: mdtext.UTF16FromByteOffset(row, start)},
				End:   Position{Line: e.SourceLine - 1, Character: mdtext.UTF16FromByteOffset(row, end)},
			},
			NewText: newSpelling,
		})
	}
}

// wikilinkStemBytes returns the byte range of the basename-stem token
// inside a `[[target#anchor|alias]]` link starting at bracketStart.
// Any folder prefix, anchor, and alias are preserved: only the stem
// after the last `/` and before `#` / `|` / `]]` is returned.
func wikilinkStemBytes(row []byte, bracketStart int) (int, int, bool) {
	i := bracketStart
	if i < 0 || i+1 >= len(row) || row[i] != '[' || row[i+1] != '[' {
		return 0, 0, false
	}
	start := i + 2
	end := start
	for end < len(row) {
		c := row[end]
		if c == '#' || c == '|' || c == ']' {
			break
		}
		end++
	}
	stemStart := start
	for j := start; j < end; j++ {
		if row[j] == '/' {
			stemStart = j + 1
		}
	}
	if stemStart >= end {
		return 0, 0, false
	}
	return stemStart, end, true
}

// wikilinkKeyHolders counts, in one pass over files, the Markdown
// files addressed by oldStem and the files holding newKey. newKey is a
// stem when newIsStem (a Markdown destination) and otherwise a
// lowercased exact basename, since a typed wikilink such as
// `[[guide.mdx]]` resolves by file name. src always counts as an
// oldStem holder, listed or not, because Resolve reads it from disk.
// files are normalized before the compare, as appendReferrerEdits does,
// so a listed `./src` is not counted a second time.
func wikilinkKeyHolders(files []string, src, oldStem, newKey string, newIsStem bool) (oldN, newN int) {
	srcListed := false
	for _, f := range files {
		if index.NormalizePath(f) == src {
			srcListed = true
		}
		stem, isMD := linkgraph.FileStemKey(path.Base(f))
		if isMD && stem == oldStem {
			oldN++
		}
		if newIsStem {
			if isMD && stem == newKey {
				newN++
			}
		} else if linkgraph.FileNameKey(path.Base(f)) == newKey {
			newN++
		}
	}
	if !srcListed {
		oldN++
	}
	return oldN, newN
}

// dstWikilinkSpelling returns the token a rewritten wikilink names dst
// by, with ok=false when no token reaches it (see
// linkgraph.WikilinkReaches). A Markdown dst is first tried as its
// basename stem in its original casing, so the link reads naturally
// (`[[Service]]`, not a lowercased match key). When the bare stem does
// not reach dst, the whole basename is tried: `[[v1.3]]` reads `.3` as
// a typed extension and `[[guide ]]` loses its space to the target
// trim, while `[[v1.3.md]]` and `[[guide .md]]` reach the file. Any
// other name is only ever spelled whole.
func dstWikilinkSpelling(dst string) (string, bool) {
	base := path.Base(dst)
	if ext := path.Ext(base); mdpath.HasMarkdownExt(ext) {
		if stem := strings.TrimSuffix(base, ext); linkgraph.WikilinkReaches(stem, base) {
			return stem, true
		}
	}
	if linkgraph.WikilinkReaches(base, base) {
		return base, true
	}
	return "", false
}
