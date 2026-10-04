package refactor

import (
	"errors"
	"path"
	"strings"
	"unicode"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdpath"
)

// ErrDuplicateSource is returned for a batch pair whose source an
// earlier pair of the same batch already moves. The earlier pair keeps
// its own verdict; this one is never planned.
var ErrDuplicateSource = errors.New("source is moved by an earlier pair of the batch")

// ErrDuplicateDestination is returned for every batch pair whose
// destination another pair of the same batch also names, in any
// letter case (a case-insensitive file system stores both as one
// file): the file that lands there last is unknown, so none of them
// is planned.
var ErrDuplicateDestination = errors.New("destination is named by another pair of the batch")

// MovePair is one relocation in a MoveAll batch: the workspace file
// Src moves to Dst. Both are workspace-relative.
type MovePair struct {
	Src, Dst string
}

// BatchMove is MoveAll's verdict on one pair. Src and Dst are the
// pair's normalized paths. Key is the source's edit key (as
// Workspace.Resolve returns it) when the source is a readable batch
// member, and empty otherwise. Err is nil for a planned pair and holds
// the reason another pair could not be planned: the same errors Move
// returns, plus ErrDuplicateSource and ErrDuplicateDestination.
type BatchMove struct {
	Src, Dst string
	Key      string
	Err      error
}

// BatchPlan is the merged result of MoveAll. Its Plan holds every
// edit, keyed per output target with one edit per range, and no
// FileOp: Moves lists each pair's relocation and verdict in request
// order. Withheld counts the links that may not reach their file once
// the batch has run. Each gets no edit, except a planned member's link
// to a file a refused member lands on, which is still re-spelled from
// the member's new folder and counted too. They are: one from a
// planned member to a member whose move could not be planned, one
// inside such a member to a planned member that stops resolving or
// whose old path another member takes, one inside such a member to
// any other file that may name another file from the member's new
// folder (see countMisread), a wikilink (a `[[stem]]` or a typed
// `[[name.ext]]`) whose new key another member's destination wins, a
// wikilink left as written that another member's destination takes
// (see stolen), and every path link and wikilink to a shadowed path
// or to a file a refused member lands on (both moveBatch.taken, see
// countShadowed), from any file but that one.
type BatchPlan struct {
	Plan
	Moves    []BatchMove
	Withheld int
}

// MoveAll plans a batch of file moves that run together, as one LSP
// workspace/willRenameFiles request performs them. It rewrites the
// same references Move does, but reads every link against the batch:
// a link from one moved file to another is spelled from the holder's
// new folder to the target's new path, so each link gets one edit,
// planned once.
//
// A file another file links to is handled by the target's incoming
// pass only when the holder stays put; a holder that moves re-spells
// all of its own links in its outbound pass, mapping each target
// through the batch. A pair's destination may be another pair's
// source, which the batch vacates, so a chain (b.md → z.md plus
// a.md → b.md) or a swap is planned.
//
// A pair that cannot be planned still counts as a batch member when its
// source is readable: the host (an editor) moves it anyway. No path
// edit is planned for a link between it and another member, except a
// link inside it when it lands in its own folder, which reads the same
// from there. A link to it from a planned member counts in Withheld,
// and so does a link inside it to a planned member when that link
// stops resolving after the batch, or when another member lands on
// that member's old path, which the link reaches if the host leaves
// the file in place. A link inside it to a file the
// batch does not plan to move counts only when the member leaves its
// folder and the link, read from there, names another file that may
// be there after the batch (see countMisread); one that stops
// resolving is left to MDS027. When another member may land on its
// path, planned or refused as a duplicate destination, every path link
// and wikilink to it counts too (see
// countShadowed): it then reaches the newcomer. When it lands on a
// file outside the batch, every path link and wikilink to that file
// counts as well: no client says whether the host overwrites it, and
// if it does, the link reaches the moved file. A planned member's link
// to that file is still re-spelled from its new folder.
//
// Move is MoveAll with one pair, except that a refused pair plans
// nothing.
func MoveAll(ws MoveWorkspace, pairs []MovePair) BatchPlan {
	moves, b := validateBatch(ws, pairs)
	return planBatch(ws, moves, b)
}

// planBatch plans the edits for moves, the verdicts validateBatch gave
// with the batch state b, and counts the links it leaves withheld.
func planBatch(ws MoveWorkspace, moves []BatchMove, b *moveBatch) BatchPlan {
	bp := BatchPlan{Plan: Plan{Edits: map[string][]Edit{}}, Moves: moves}
	p := lint.NewParser()
	r := &destResolver{ws: ws, batch: b}
	appendReferrerEdits(bp.Edits, ws, p, r)
	for _, m := range moves {
		if m.Err != nil {
			continue
		}
		appendWikilinkKeyEdits(bp.Edits, ws, r, m.Src, m.Dst)
		if mdpath.HasMarkdownExt(path.Ext(m.Src)) || r.listed(m.Src) {
			appendOutboundEdits(bp.Edits, p, r, m.Key, m.Src, m.Dst, b.sources[m.Src])
		}
	}
	for p := range b.taken {
		countShadowed(ws, r, p)
	}
	stableSortEdits(bp.Edits)
	bp.Withheld = b.withheld
	return bp
}

// countShadowed counts, in the batch, every wikilink to vacated, a path
// a newcomer may take (moveBatch.taken): a member whose move was
// refused, whose path another member may take, or a file outside the
// batch a refused member lands on. The host still moves vacated, or may
// replace it, so each link to it then reaches the newcomer; it still
// resolves, so no rule flags it, and the batch plans no edit for it. A
// wikilink is counted when its key reaches vacated today: a `[[stem]]`
// link for a Markdown file, a typed `[[name.ext]]` link for any other
// (see wikilinkKey). A replaced file's link to itself is not counted
// (see moveBatch.replaced): once replaced, the file holding it is gone. A path link to
// it is counted in the referrer scan (see appendReferrerEdits), or, in
// a planned member, by its outbound pass (see outboundEdit), so the
// workspace is still read once.
func countShadowed(ws MoveWorkspace, r *destResolver, vacated string) {
	if !linkgraph.WikilinkIndexed(vacated) {
		return
	}
	k := fileWikilinkKey(vacated)
	edges := k.edges(ws)
	if len(edges) == 0 || !k.resolvesTo(r.wikilinkIndex(), vacated) {
		return
	}
	lines := r.edgeReader()
	for _, e := range edges {
		if r.batch.replaced(vacated) && index.NormalizePath(e.SourceFile) == vacated {
			continue
		}
		if _, row, ok := lines.row(e); ok {
			if _, _, ok := k.at(row, e.SourceCol-1); ok {
				r.batch.withheld++
			}
		}
	}
}

// validateBatch normalizes every pair, records each readable source as
// a batch member, and gives each pair its verdict. A pair is planned
// when both paths stay in the workspace, they differ, no earlier pair
// moves its source, no other member lands on its destination, and its
// destination is free once the batch has run: absent from the
// workspace or another member's source.
// A refused member whose path another member lands on is recorded as
// shadowed, whether that lander's move is planned or refused as a
// duplicate destination: the host may move it there either way.
func validateBatch(ws MoveWorkspace, pairs []MovePair) ([]BatchMove, *moveBatch) {
	moves := make([]BatchMove, len(pairs))
	b := newMoveBatch()
	landing := map[string]int{}
	for i, pr := range pairs {
		moves[i] = b.admit(ws, pr, landing)
	}
	for i := range moves {
		m := &moves[i]
		if m.Err != nil {
			continue
		}
		_, vacated := b.members[m.Dst]
		exists := !vacated && resolves(ws, m.Dst)
		switch {
		case landing[foldPath(m.Dst)] > 1:
			m.Err = ErrDuplicateDestination
		case exists:
			m.Err = DestinationExistsError{Dst: m.Dst}
		default:
			b.members[m.Src] = batchMember{dst: m.Dst, planned: true}
		}
		if exists {
			// Every pair whose destination exists is refused above.
			b.taken[m.Dst] = true
		}
	}
	for _, m := range moves {
		lander, moved := b.members[m.Src]
		if u, ok := b.members[m.Dst]; moved && lander.dst == m.Dst && ok && !u.planned {
			b.taken[m.Dst] = true
		}
	}
	return moves, b
}

// admit normalizes pr and records its source as a batch member when it
// is readable, recording the member's destination in b.dsts and
// counting it in landing, keyed by foldPath. The returned move carries
// the checks Move runs first, in Move's order: a traversal path, an
// equal source and destination, then a missing source; a source an
// earlier pair moves fails as well. A pair that passes still awaits
// the destination checks in validateBatch.
func (b *moveBatch) admit(ws MoveWorkspace, pr MovePair, landing map[string]int) BatchMove {
	m := BatchMove{Src: index.NormalizePath(pr.Src), Dst: index.NormalizePath(pr.Dst)}
	_, seen := b.members[m.Src]
	switch {
	case !workspaceRelative(m.Src):
		m.Err = ErrTraversalPath
		return m
	case m.Src == m.Dst:
		m.Err = ErrSameFile
		return m
	case seen:
		m.Err = ErrDuplicateSource
		return m
	}
	key, source, ok := ws.Resolve(m.Src)
	dstOK := workspaceRelative(m.Dst)
	if ok {
		m.Key = key
		b.sources[m.Src] = source
		member := batchMember{}
		if dstOK {
			member.dst = m.Dst
			b.dsts[m.Dst] = true
			landing[foldPath(m.Dst)]++
		}
		b.members[m.Src] = member
	}
	switch {
	case !dstOK:
		m.Err = ErrTraversalPath
	case !ok:
		m.Err = SourceNotFoundError{Src: m.Src}
	}
	return m
}

// resolves reports whether ws can read the file p.
func resolves(ws MoveWorkspace, p string) bool {
	_, _, ok := ws.Resolve(p)
	return ok
}

// moveBatch is the shared state of one MoveAll run: every member's new
// path and whether its move was planned, the paths a newcomer may
// take, and the count of links left stale without an edit.
type moveBatch struct {
	members map[string]batchMember
	sources map[string][]byte // each member's text, as admit read it
	dsts    map[string]bool   // every member's dst, as admit records it
	// taken holds each path a newcomer may take once the batch has
	// run, so every link to it is counted (see countShadowed): a
	// refused member another member lands on, which the host moves
	// away, or a file outside the batch a refused member lands on,
	// which the host may replace (see replaced).
	taken    map[string]bool
	withheld int
	post     *linkgraph.WikilinkIndex // postIndex, built on first use
	keys     map[wikilinkKey]int      // keyHolders, built by buildKeys
	srcKeys  map[wikilinkKey][]string // keySources, built by buildKeys
}

// replaced reports whether the taken path p is a file outside the
// batch, which a refused member may replace, rather than a member the
// host moves away. A replaced file's link to itself is not counted:
// once replaced, the file holding it is gone. A moved member's is,
// unless it still names the file from where the host puts it.
func (b *moveBatch) replaced(p string) bool {
	_, member := b.members[p]
	return b.taken[p] && !member
}

// keyHolders returns how many members hold the wikilink key k (a stem
// or an exact name, see wikilinkKey) with their source or their
// destination, each member counted once.
func (b *moveBatch) keyHolders(k wikilinkKey) int {
	b.buildKeys()
	return b.keys[k]
}

// keySources returns every member source whose wikilink key is k, in
// no set order.
func (b *moveBatch) keySources(k wikilinkKey) []string {
	b.buildKeys()
	return b.srcKeys[k]
}

// buildKeys builds the wikilink keys keyHolders and keySources read.
// It runs on the first call, once every verdict is in, so a batch
// reads its members once, not once per move.
func (b *moveBatch) buildKeys() {
	if b.keys != nil {
		return
	}
	b.keys, b.srcKeys = map[wikilinkKey]int{}, map[wikilinkKey][]string{}
	for src, m := range b.members {
		s := fileWikilinkKey(src)
		b.keys[s]++
		b.srcKeys[s] = append(b.srcKeys[s], src)
		if m.dst == "" {
			continue
		}
		if d := fileWikilinkKey(m.dst); d != s {
			b.keys[d]++
		}
	}
}

// newMoveBatch returns an empty batch, ready for admit.
func newMoveBatch() *moveBatch {
	return &moveBatch{
		members: map[string]batchMember{}, sources: map[string][]byte{}, dsts: map[string]bool{},
		taken: map[string]bool{},
	}
}

// batchMember is one moved file: dst is where the host puts it (empty
// when that is outside the workspace), and planned reports whether
// MoveAll planned its move.
type batchMember struct {
	dst     string
	planned bool
}

// scanBases returns, each once, the base name of every planned
// member's source and of every taken path: the names a link the
// referrer scan reads spells out.
func (b *moveBatch) scanBases() [][]byte {
	seen := map[string]bool{}
	var bases [][]byte
	add := func(p string) {
		if base := path.Base(p); !seen[base] {
			seen[base] = true
			bases = append(bases, []byte(base))
		}
	}
	for src, m := range b.members {
		if m.planned {
			add(src)
		}
	}
	for p := range b.taken {
		add(p)
	}
	return bases
}

// member returns the batch entry for the workspace file p.
func (r *destResolver) member(p string) (batchMember, bool) {
	m, ok := r.batch.members[p]
	return m, ok
}

// countStale counts, in the batch, one link that no edit rewrites:
// written as refPath in a file landing at holder, it should name the
// file landing at target. It counts only when that link stops
// resolving, or when either end leaves the workspace.
func (r *destResolver) countStale(holder, refPath, target string) {
	if holder == "" || target == "" || linkgraph.ResolveRelTarget(holder, refPath) != target {
		r.batch.withheld++
	}
}

// foldPath returns p with every rune replaced by the smallest rune of
// its simple case-folding orbit, so two paths get one key exactly when
// strings.EqualFold reads them as equal: the spellings a
// case-insensitive file system may store as one file.
func foldPath(p string) string {
	return strings.Map(func(r rune) rune {
		lo := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			lo = min(lo, f)
		}
		return lo
	}, p)
}
