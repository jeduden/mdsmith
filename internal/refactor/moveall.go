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
// order. Withheld counts the links that get no edit yet may not
// reach their file once the batch has run: one from a planned member
// to a member whose move could not be planned, one inside such a
// member to a planned member that stops resolving, a `[[stem]]` whose
// new key another member's destination wins, and every path link and
// `[[stem]]` link to a shadowed path (see countShadowed). A typed
// `[[name.ext]]` link to a shadowed non-Markdown file is not counted:
// the index has no edge lookup for it (plan 2610040606).
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
// stops resolving after the batch. A link inside it to a file the
// batch leaves in place is not counted: MDS027 flags it if it stops
// resolving. When a planned member lands on its path, every path link
// and `[[stem]]` link to it counts too (see countShadowed): it then
// reaches the newcomer.
//
// Move is MoveAll with one pair.
func MoveAll(ws Workspace, pairs []MovePair) BatchPlan {
	moves, b := validateBatch(ws, pairs)
	bp := BatchPlan{Plan: Plan{Edits: map[string][]Edit{}}, Moves: moves}
	p := lint.NewParser()
	r := &destResolver{ws: ws, batch: b}
	appendReferrerEdits(bp.Edits, ws, p, r)
	for _, m := range moves {
		if m.Err != nil {
			continue
		}
		appendWikilinkStemEdits(bp.Edits, ws, r, m.Src, m.Dst)
		if mdpath.HasMarkdownExt(path.Ext(m.Src)) || r.listed(m.Src) {
			appendOutboundEdits(bp.Edits, p, r, m.Key, m.Src, m.Dst, b.sources[m.Src])
		}
	}
	for _, m := range moves {
		if m.Err == nil && b.shadowed[m.Dst] {
			countShadowed(ws, r, m.Dst)
		}
	}
	stableSortEdits(bp.Edits)
	bp.Withheld = b.withheld
	return bp
}

// countShadowed counts, in the batch, every `[[stem]]` link to
// vacated: a member whose move was refused, whose path a planned
// member takes (moveBatch.shadowed). The host still moves vacated, so
// each link to it then reaches the newcomer; it still resolves, so no
// rule flags it, and the batch plans no edit for it. A `[[stem]]` link
// is counted when its stem reaches vacated today. A path link to it is
// counted in the referrer scan (see appendReferrerEdits), or, in a
// planned member, by its outbound pass (see outboundEdit), so the
// workspace is still read once.
func countShadowed(ws Workspace, r *destResolver, vacated string) {
	stem, ok := linkgraph.FileStemKey(path.Base(vacated))
	if !ok || !linkgraph.WikilinkIndexed(vacated) {
		return
	}
	edges := ws.IncomingWikilinkEdges(stem)
	if len(edges) == 0 || !r.wikilinkIndex().StemResolvesTo(stem, vacated) {
		return
	}
	lines := r.edgeReader()
	for _, e := range edges {
		if _, row, ok := lines.row(e); ok {
			if got, _, _, ok := linkgraph.WikilinkStemAt(row, e.SourceCol-1); ok && got == stem {
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
func validateBatch(ws Workspace, pairs []MovePair) ([]BatchMove, *moveBatch) {
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
		switch {
		case landing[foldPath(m.Dst)] > 1:
			m.Err = ErrDuplicateDestination
		case !vacated && resolves(ws, m.Dst):
			m.Err = DestinationExistsError{Dst: m.Dst}
		default:
			b.members[m.Src] = batchMember{dst: m.Dst, planned: true}
		}
	}
	for _, m := range moves {
		if u, ok := b.members[m.Dst]; m.Err == nil && ok && !u.planned {
			b.shadowed[m.Dst] = true
		}
	}
	return moves, b
}

// admit normalizes pr and records its source as a batch member when it
// is readable, recording the member's destination in b.dsts and
// counting it in landing, keyed by foldPath. The returned move carries the checks Move runs first, in Move's order: a
// traversal path, an equal source and destination, then a missing
// source; a source an earlier pair moves fails as well. A pair that
// passes still awaits the destination checks in validateBatch.
func (b *moveBatch) admit(ws Workspace, pr MovePair, landing map[string]int) BatchMove {
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
func resolves(ws Workspace, p string) bool {
	_, _, ok := ws.Resolve(p)
	return ok
}

// moveBatch is the shared state of one MoveAll run: every member's new
// path and whether its move was planned, the refused members whose
// path a planned member takes, and the count of links left stale
// without an edit.
type moveBatch struct {
	members  map[string]batchMember
	sources  map[string][]byte // each member's text, as admit read it
	dsts     map[string]bool   // every member's dst, as admit records it
	shadowed map[string]bool   // see countShadowed
	withheld int
	post     *linkgraph.WikilinkIndex // postIndex, built on first use
	stems    map[string]int           // stemHolders, built by keyStems
	srcStems map[string][]string      // stemSources, built by keyStems
}

// stemHolders returns how many members hold the stem key stem with
// their source or their destination, each member counted once.
func (b *moveBatch) stemHolders(stem string) int {
	b.keyStems()
	return b.stems[stem]
}

// stemSources returns every member source whose stem key is stem, in
// no set order.
func (b *moveBatch) stemSources(stem string) []string {
	b.keyStems()
	return b.srcStems[stem]
}

// keyStems builds the stem keys stemHolders and stemSources read. It
// runs on the first call, once every verdict is in, so a batch reads
// its members once, not once per move.
func (b *moveBatch) keyStems() {
	if b.stems != nil {
		return
	}
	b.stems, b.srcStems = map[string]int{}, map[string][]string{}
	for src, m := range b.members {
		s, ok := linkgraph.FileStemKey(path.Base(src))
		if ok {
			b.stems[s]++
			b.srcStems[s] = append(b.srcStems[s], src)
		}
		if m.dst == "" {
			continue
		}
		if d, dok := linkgraph.FileStemKey(path.Base(m.dst)); dok && (!ok || d != s) {
			b.stems[d]++
		}
	}
}

// newMoveBatch returns an empty batch, ready for admit.
func newMoveBatch() *moveBatch {
	return &moveBatch{
		members: map[string]batchMember{}, sources: map[string][]byte{}, dsts: map[string]bool{},
		shadowed: map[string]bool{},
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
// member's source and of every shadowed path: the names a link the
// referrer scan reads spells out.
func (b *moveBatch) scanBases() [][]byte {
	seen := map[string]bool{}
	var bases [][]byte
	for src, m := range b.members {
		if base := path.Base(src); (m.planned || b.shadowed[src]) && !seen[base] {
			seen[base] = true
			bases = append(bases, []byte(base))
		}
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
