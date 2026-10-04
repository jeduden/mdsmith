package refactor

import (
	"bytes"
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
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
//     stem, outside `.git` and `node_modules`, is a stem target, and a
//     dst no wikilink can name — no extension, an empty stem, a `#`,
//     `|`, `[`, `]`, backtick, CR, or newline in the name, a name that
//     ends with a space, or a path under `.git` or `node_modules`,
//     which the resolver skips — gets no rewrite. A name that starts
//     with a space or reads as a drive path (`C:x.md`) is written
//     behind `./`;
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
//
// Move is MoveAll with one pair; MoveAll plans several moves that run
// together.
func Move(ws Workspace, src, dst string) (Plan, error) {
	bp := MoveAll(ws, []MovePair{{Src: src, Dst: dst}})
	m := bp.Moves[0]
	if m.Err != nil {
		return Plan{}, m.Err
	}
	return Plan{Edits: bp.Edits, FileOp: &FileOp{From: m.Src, To: m.Dst}}, nil
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

// appendReferrerEdits repoints every destination in a workspace file
// that names a planned batch member — inline links, images, and
// reference definitions — so it names the member's new path. The
// workspace is scanned once for the whole batch. A planned member's
// own file is skipped: its outbound pass spells every link in it,
// including one to itself, from its new folder. A holder whose move
// could not be planned gets no edit unless the host keeps it in its
// folder, where the link is spelled from the same directory whether or
// not the move runs; otherwise the link counts as withheld when it
// stops resolving. The same scan counts each link to a shadowed path
// (see countShadowed) from any file but that path's own and a planned
// member's.
//
// Every file is read, but only one mayNameAny admits is parsed. The
// index is not consulted: it records no edge for an image or a
// ref-def, and it reads a literal `what?.md` as `what`.
func appendReferrerEdits(changes map[string][]Edit, ws Workspace, p parser.Parser, r *destResolver) {
	bases := r.batch.scanBases()
	if len(bases) == 0 {
		return
	}
	for _, rel := range r.paths() {
		holder, moved := r.member(rel)
		if moved && holder.planned {
			continue
		}
		key, source, ok := ws.Resolve(rel)
		if !ok || !mayNameAny(source, bases) {
			continue
		}
		for _, d := range locateDests(p, rel, source) {
			if edit, ok := r.referrerEdit(d, rel, holder, moved); ok {
				changes[key] = append(changes[key], edit)
			}
		}
	}
}

// referrerEdit is appendReferrerEdits for one destination d in the
// workspace file rel, whose batch entry is holder when moved. It
// returns the edit that repoints d at a planned member's new path, and
// counts d instead when it names a shadowed path (see countShadowed)
// or when holder's refused move takes d out of the folder it is
// spelled from.
func (r *destResolver) referrerEdit(d inlineDest, rel string, holder batchMember, moved bool) (Edit, bool) {
	ref, ok := r.target(rel, d.dest)
	if !ok {
		return Edit{}, false
	}
	if r.batch.shadowed[ref.target] {
		// The newcomer takes the path, so a link to it is counted. A
		// shadowed file's link to itself is counted too, unless it
		// still names the file from where the host moves it.
		if ref.target != rel {
			r.batch.withheld++
		} else {
			r.countStale(holder.dst, ref.path, holder.dst)
		}
		return Edit{}, false
	}
	tgt, isMember := r.member(ref.target)
	if !isMember || !tgt.planned {
		return Edit{}, false
	}
	if moved && !unplannedInPlace(holder, rel) {
		r.countStale(holder.dst, ref.path, tgt.dst)
		return Edit{}, false
	}
	return destEdit(d, ref, rel, tgt.dst)
}

// mayNameAny reports whether source may hold a destination that names
// a file with one of the base names bases. It needs a `](` or `]:` to
// open one, and a base written out or a `%` that may escape it: a
// destination's path, once decoded and cleaned, ends in the base name
// it names, and cleaning only drops path segments. The link marks and
// the `%` do not depend on the base, so each is looked for once, not
// once per base.
func mayNameAny(source []byte, bases [][]byte) bool {
	if len(bases) == 0 || !bytes.Contains(source, linkMark) && !bytes.Contains(source, refDefMark) {
		return false
	}
	if bytes.IndexByte(source, '%') >= 0 {
		return true
	}
	for _, base := range bases {
		if bytes.Contains(source, base) {
			return true
		}
	}
	return false
}

// unplannedInPlace reports whether m, the batch entry for the file rel,
// is a move that could not be planned and lands in rel's own folder.
// A link in such a file is read from the same directory before and
// after the host moves it, so an edit spelled from rel stays right.
func unplannedInPlace(m batchMember, rel string) bool {
	return !m.planned && m.dst != "" && path.Dir(m.dst) == path.Dir(rel)
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
	// A target the batch also moves is named at its new path; one whose
	// move could not be planned gets no edit, and the warning counts
	// the link. No spelling is right both ways: the old path breaks if
	// the host does move it, and its destination may name the file a
	// refused overwrite leaves there. Left as written, the link either
	// stops resolving, where MDS027 flags it, or reaches that
	// destination, which is right only if the host overwrites it — so
	// it is counted even then.
	if m, moved := r.member(tgt); tgt == src {
		tgt = dst
	} else if moved {
		if !m.planned {
			r.batch.withheld++
			return Edit{}, false
		}
		tgt = m.dst
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

// destResolver reads destinations for a move of src. It also holds the
// workspace file list, read once per move and normalized, which the
// referrer scan, the listed checks, and the wikilink same-stem guard of
// a workspace with no wikilink index share.
type destResolver struct {
	ws    Workspace
	batch *moveBatch // the MoveAll batch; a lone Move is a batch of one
	list  []string   // nil until paths first runs; never nil after
	files map[string]bool

	wl     *linkgraph.WikilinkIndex // ws.WikilinkIndex, once wlRead
	wlRead bool
}

// paths returns the workspace's files, normalized as Resolve keys
// them. It reads ws.Files() once, on the first call.
func (r *destResolver) paths() []string {
	if r.list == nil {
		files := r.ws.Files()
		r.list = make([]string, len(files))
		for i, f := range files {
			r.list[i] = index.NormalizePath(f)
		}
	}
	return r.list
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

// exists reports whether p names a batch member or another file the
// workspace lists.
func (r *destResolver) exists(p string) bool {
	_, moved := r.member(p)
	return moved || r.listed(p)
}

// listed reports whether the workspace lists p. It builds the lookup
// set once, on the first call.
func (r *destResolver) listed(p string) bool {
	if r.files == nil {
		list := r.paths()
		r.files = make(map[string]bool, len(list))
		for _, f := range list {
			r.files[f] = true
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

// appendWikilinkStemEdits rewrites the basename segment of each
// `[[old-stem]]` link to the token dstWikilinkSpelling picks for dst:
// the new stem, the whole basename, or either behind `./`. It runs only
// when `[[old-stem]]` would stop reaching dst: a Markdown dst with
// another stem, or a dst with a non-Markdown name. A move that keeps the
// stem leaves wikilinks alone, since no other spelling would serve
// better. With a same-stem sibling, though, the new path can sort on
// the other side of it, and every `[[stem]]` then reaches the other
// file of the two. In a batch, a kept stem another member also holds
// is read against the batch (see keptStemTarget).
func appendWikilinkStemEdits(changes map[string][]Edit, ws Workspace, r *destResolver, src, dst string) {
	// Both ends are keyed the way NewWikilinkIndex keys files. Only a
	// Markdown src has a stem key, so moving any other file retargets
	// no `[[stem]]` link. An empty src key (`docs/.md`) matches no edge,
	// since no target spells it, so the edge lookup below returns early.
	// ` guide.md` keys as " guide": a bare `[[guide]]` never reached it,
	// while a folder-prefixed `[[x/ guide]]` did and is rewritten.
	// A src under `.git` or `node_modules` is never indexed, so no
	// `[[oldStem]]` link reached it: one that resolved elsewhere or
	// nowhere must not be retargeted at dst.
	oldStem, ok := linkgraph.FileStemKey(path.Base(src))
	if !ok || !linkgraph.WikilinkIndexed(src) {
		return
	}
	// A Markdown destination is addressed by stem, so keeping the stem
	// keeps every link resolving. A typed destination (`guide.png`) is
	// addressed by exact name, a different key space, so it always needs
	// the rewrite: comparing its name to oldStem would wrongly skip a
	// move such as docs/guide.png.md → docs/guide.png.
	//
	// A Markdown destination that keeps the stem needs no rewrite of its
	// own, but in a batch another member can still change what
	// `[[oldStem]]` reaches (see keptStemTarget): its links are then
	// read against the batch too, and rewritten only to follow a named
	// sibling.
	self, rewrite := newStemTarget(oldStem, dst)
	if !rewrite {
		var kept bool
		if self, kept = r.keptStemTarget(oldStem, dst); !kept {
			return
		}
	}
	// A wikilink resolves by basename stem alone: every `[[oldStem]]`,
	// with or without a folder prefix such as `[[ref/Guide]]`, reaches
	// the same-stem file that sorts first (shallowest, then by name).
	// When that file is a sibling that is not moving, every such link
	// reaches the sibling and stays as written. When it is src, every
	// such link reaches src today and would silently reach a sibling
	// once src is gone, so all of them are rewritten. The files are
	// read from ws.WikilinkIndex, the set the resolver reads, listed or
	// not. src counts as a holder even when that index lacks it.
	//
	// A link whose folder prefix names a sibling's folder was written
	// for that sibling, so it is left as written — unless the batch
	// moves the sibling to a new stem too, when it follows the sibling.
	//
	// Each rewrite must reach its file once the batch has run (see
	// stemTarget.reaches), read from the index with every batch move
	// applied. When a file outside the batch takes the new key, the
	// link is left alone, as a lone move always has; when another
	// batch member's destination takes it, the link is counted as
	// withheld.
	//
	// Most moves have no `[[oldStem]]` link at all, so the edges are
	// fetched first and the workspace walk that builds the index is
	// skipped.
	edges := ws.IncomingWikilinkEdges(oldStem)
	if len(edges) == 0 {
		return
	}
	idx := r.wikilinkIndex()
	if !r.winsStem(idx, oldStem, src) {
		return
	}
	post := r.postIndex(idx)
	siblings := stemSiblings(idx.StemPaths(oldStem), src)
	lines := edgeLines{ws: ws}
	for _, e := range edges {
		key, row, ok := lines.row(e)
		if !ok {
			continue
		}
		// An edge from a stale index can point at a column that now
		// holds a link to another file; only a link still keyed by
		// oldStem is rewritten.
		stem, start, end, ok := linkgraph.WikilinkStemAt(row, e.SourceCol-1)
		if !ok || stem != oldStem {
			continue
		}
		t, needed := self, rewrite
		if len(siblings) > 0 {
			if sib, named := wikilinkNamedSibling(row[e.SourceCol-1:start], src, siblings); named {
				if t, needed = r.siblingTarget(oldStem, sib); !needed {
					continue
				}
			}
		}
		if !t.reaches(post) {
			r.countBlocked(post, t)
			continue
		}
		if !needed {
			// dst keeps the stem and still wins it.
			continue
		}
		text := t.spelling
		// A folder prefix already keeps the name from reading as a
		// drive path, so the `./` guard is only for a bare link. The
		// span starts after the `[[`, so start-1 is always in the row.
		if t.needsPrefix && row[start-1] != '/' && row[start-1] != '\\' {
			text = "./" + text
		}
		changes[key] = append(changes[key], Edit{
			Range: Range{
				Start: Position{Line: e.SourceLine - 1, Character: mdtext.UTF16FromByteOffset(row, start)},
				End:   Position{Line: e.SourceLine - 1, Character: mdtext.UTF16FromByteOffset(row, end)},
			},
			NewText: text,
		})
	}
}

// stemTarget is the file a rewritten `[[stem]]` link names after a
// move: dst, the token that names it (see dstWikilinkSpelling), and
// key, the stem (isStem) or lowercased exact name the resolver looks
// it up by.
type stemTarget struct {
	dst         string
	spelling    string
	needsPrefix bool
	key         string
	isStem      bool
}

// newStemTarget returns the stemTarget for a move of a file keyed by
// oldStem to dst. ok is false when `[[oldStem]]` still reaches dst
// (a Markdown dst with the same stem), when no token reaches dst (see
// linkgraph.WikilinkReaches), or when dst sits under `.git` or
// `node_modules`, which the resolver never indexes.
func newStemTarget(oldStem, dst string) (stemTarget, bool) {
	newStem, isStem := linkgraph.FileStemKey(path.Base(dst))
	if isStem && oldStem == newStem {
		return stemTarget{}, false
	}
	spelling, needsPrefix, ok := dstWikilinkSpelling(dst, isStem)
	if !ok || !linkgraph.WikilinkIndexed(dst) {
		return stemTarget{}, false
	}
	key := newStem
	if !isStem {
		key = linkgraph.FileNameKey(path.Base(dst))
	}
	return stemTarget{dst: dst, spelling: spelling, needsPrefix: needsPrefix, key: key, isStem: isStem}, true
}

// reaches reports whether a link keyed by t.key resolves to t.dst in
// post, the index as it reads once the move (or batch) has run. A
// Markdown destination is looked up by stem; a typed non-Markdown one
// (`guide.mdx`) by exact file name. dst counts as a holder even when
// post lacks it. post holds dst itself, so a holder spelling dst in
// another letter case alone is checked here: on a case-insensitive
// file system it may be dst, and then dst is not known to win.
func (t stemTarget) reaches(post *linkgraph.WikilinkIndex) bool {
	if slices.ContainsFunc(t.holders(post), func(q string) bool { return q != t.dst && strings.EqualFold(q, t.dst) }) {
		return false
	}
	if t.isStem {
		return post.StemResolvesTo(t.key, t.dst)
	}
	return post.NameResolvesTo(t.key, t.dst)
}

// holders returns the files in post that a link keyed by t.key
// reaches, in resolver order: the stem holders for a Markdown
// destination, the exact-name holders for a typed one.
func (t stemTarget) holders(post *linkgraph.WikilinkIndex) []string {
	if t.isStem {
		return post.StemPaths(t.key)
	}
	return post.NamePaths(t.key)
}

// keptStemTarget returns the target a `[[oldStem]]` link keeps when the
// move of src to dst keeps that stem: dst, keyed by oldStem. ok is true
// only in a batch where another member's source or destination also
// holds oldStem, the one way the batch can change what such a link
// reaches: a destination that outsorts dst takes every bare link (see
// countBlocked), and a renamed sibling takes the links that name it
// (see siblingTarget). A lone move, or a batch with no such member,
// leaves the links alone.
func (r *destResolver) keptStemTarget(oldStem, dst string) (stemTarget, bool) {
	if !linkgraph.WikilinkIndexed(dst) {
		return stemTarget{}, false
	}
	if stem, isStem := linkgraph.FileStemKey(path.Base(dst)); !isStem || stem != oldStem {
		return stemTarget{}, false
	}
	// The moving file is a member holding oldStem itself, so another
	// member holds it too when the count passes one.
	if r.batch.stemHolders(oldStem) < 2 {
		return stemTarget{}, false
	}
	return stemTarget{dst: dst, key: oldStem, isStem: true}, true
}

// siblingTarget returns the target a link naming the sibling sib by
// folder follows: sib's new name when the batch moves it to a new
// stem. ok is false when sib stays put, keeps its stem, or its move
// was not planned, and the link is then left as written.
func (r *destResolver) siblingTarget(oldStem, sib string) (stemTarget, bool) {
	m, moved := r.member(sib)
	if !moved || !m.planned {
		return stemTarget{}, false
	}
	return newStemTarget(oldStem, m.dst)
}

// countBlocked counts, in the batch, a `[[stem]]` rewrite to t that
// is not planned because another batch member's destination wins
// t.key in post. A file outside the batch that wins it, or that
// spells t.dst in another letter case alone, is not counted: a lone
// move leaves such a link alone too. No member destination spells
// t.dst in another case: validateBatch refuses both such pairs, and t
// is always a planned member's destination.
func (r *destResolver) countBlocked(post *linkgraph.WikilinkIndex, t stemTarget) {
	// t does not reach its file in post, so some other file holds
	// t.key there: it sorts first, or it is t.dst in another case.
	if first := t.holders(post)[0]; first != t.dst && r.batch.dsts[first] {
		r.batch.withheld++
	}
}

// winsStem reports whether a `[[oldStem]]` link reaches src before the
// batch runs: src must sort before idx's holders and before every
// other member source holding oldStem, which idx may lack (a file
// under a symlinked or unreadable directory the walk skips). Without
// the members, two sources idx lacks would each win the stem and plan
// two edits for each link.
func (r *destResolver) winsStem(idx *linkgraph.WikilinkIndex, oldStem, src string) bool {
	if !idx.StemResolvesTo(oldStem, src) {
		return false
	}
	members := r.batch.stemSources(oldStem)
	return len(members) < 2 || linkgraph.NewWikilinkIndexFromPaths(members).StemResolvesTo(oldStem, src)
}

// wikilinkIndex returns ws.WikilinkIndex, read once per resolver. A
// workspace whose root walk failed has no index; the files it lists
// are then the best known set, so a listed sibling still blocks a
// rewrite.
func (r *destResolver) wikilinkIndex() *linkgraph.WikilinkIndex {
	if !r.wlRead {
		r.wl, r.wlRead = r.ws.WikilinkIndex(), true
		if r.wl == nil {
			r.wl = linkgraph.NewWikilinkIndexFromPaths(r.paths())
		}
	}
	return r.wl
}

// postIndex returns idx as it reads once the batch has run: every
// member's source removed and its destination added. It is built once
// per batch.
func (r *destResolver) postIndex(idx *linkgraph.WikilinkIndex) *linkgraph.WikilinkIndex {
	if r.batch.post == nil {
		moves := make(map[string]string, len(r.batch.members))
		for s, m := range r.batch.members {
			moves[s] = m.dst
		}
		r.batch.post = idx.Moved(moves)
	}
	return r.batch.post
}

// stemSiblings returns the holders of the moved file's stem other than
// src, in a slice of its own: the files a `[[stem]]` folder prefix can
// name in its place.
func stemSiblings(holders []string, src string) []string {
	var out []string
	for _, h := range holders {
		if h != src {
			out = append(out, h)
		}
	}
	return out
}

// wikilinkNamedSibling returns the one of siblings whose folder a
// wikilink names by its folder prefix, when that prefix does not name
// src's folder too. lead is the link's text from its `[[` up to its
// base segment. The resolver reads the basename alone, so such a link
// reaches src while src sorts first, but its author named the sibling,
// and it reaches that file once src is gone. A prefix names a file's
// folder when it equals the folder's trailing path segments, ignoring
// case and reading `\` as `/`; a bare link names none. The first
// sibling named, in resolver order, is returned.
func wikilinkNamedSibling(lead []byte, src string, siblings []string) (string, bool) {
	lead = bytes.TrimPrefix(lead, []byte("[["))
	folder := path.Clean(strings.ReplaceAll(strings.TrimSpace(string(lead)), `\`, "/"))
	if folder == "." || folderNames(folder, src) {
		return "", false
	}
	for _, s := range siblings {
		if folderNames(folder, s) {
			return s, true
		}
	}
	return "", false
}

// folderNames reports whether folder equals the trailing segments of
// file's folder, ignoring case.
func folderNames(folder, file string) bool {
	dir := strings.ToLower(path.Dir(file))
	folder = strings.ToLower(folder)
	return dir == folder || strings.HasSuffix(dir, "/"+folder)
}

// dstWikilinkSpelling returns the token a rewritten wikilink names dst
// by, with ok=false when no token reaches it (see
// linkgraph.WikilinkReaches). dstIsMarkdown is FileStemKey's answer for
// dst's basename. A Markdown dst is first tried as its basename stem in
// its original casing, so the link reads naturally (`[[Service]]`, not
// a lowercased match key). When the bare stem does not reach dst, the
// whole basename is tried: `[[v1.3]]` reads `.3` as a typed extension
// and `[[guide ]]` loses its space to the target trim, while
// `[[v1.3.md]]` and `[[guide .md]]` reach the file. Any other name is
// only ever spelled whole.
//
// A name the resolver refuses as a drive path (`C:x.md`) or trims bare
// reaches dst only behind a `./`, which the resolver drops when it
// reads the basename. The spelling is then returned without the `./`
// and needsPrefix is true: a link that already has a folder prefix
// writes it as is, any other writes `./` first.
func dstWikilinkSpelling(dst string, dstIsMarkdown bool) (spelling string, needsPrefix, ok bool) {
	base := path.Base(dst)
	var candidates [2]string
	n := 0
	if dstIsMarkdown {
		candidates[n] = strings.TrimSuffix(base, path.Ext(base))
		n++
	}
	candidates[n] = base
	n++
	for _, prefix := range [...]string{"", "./"} {
		for _, c := range candidates[:n] {
			if linkgraph.WikilinkReaches(prefix+c, base) {
				return c, prefix != "", true
			}
		}
	}
	return "", false, false
}
