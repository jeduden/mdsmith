package refactor

import (
	"errors"
	"path"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdpath"
)

// ErrDuplicateSource is returned for a batch pair whose source an
// earlier pair of the same batch already moves. The earlier pair is
// planned; this one is not.
var ErrDuplicateSource = errors.New("source is moved by an earlier pair of the batch")

// ErrDuplicateDestination is returned for every batch pair whose
// destination another pair of the same batch also names: the file
// that lands there last is unknown, so none of them is planned.
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
// order. StemEdits is the `[[stem]]` subset of the edits, keyed the
// same way. Withheld counts the links between two batch members that
// stop resolving yet get no edit, because one end's move could not be
// planned or because no `[[stem]]` spelling reaches the target once
// the batch has run.
type BatchPlan struct {
	Plan
	StemEdits map[string][]Edit
	Moves     []BatchMove
	Withheld  int
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
// edit is planned for a link between it and another member; such a
// link counts in Withheld when it stops resolving after the batch.
//
// Move is MoveAll with one pair.
func MoveAll(ws Workspace, pairs []MovePair) BatchPlan {
	moves, b := validateBatch(ws, pairs)
	bp := BatchPlan{
		Plan:      Plan{Edits: map[string][]Edit{}},
		StemEdits: map[string][]Edit{},
		Moves:     moves,
	}
	p := lint.NewParser()
	r := &destResolver{ws: ws, batch: b}
	for _, m := range moves {
		if m.Err != nil {
			continue
		}
		r.src = m.Src
		appendReferrerEdits(bp.Edits, ws, p, r, m.Src, m.Dst)
		appendWikilinkStemEdits(bp.StemEdits, ws, r, m.Src, m.Dst)
		if _, source, ok := ws.Resolve(m.Src); ok && (mdpath.HasMarkdownExt(path.Ext(m.Src)) || r.listed(m.Src)) {
			appendOutboundEdits(bp.Edits, p, r, m.Key, m.Src, m.Dst, source)
		}
	}
	for key, edits := range bp.StemEdits {
		bp.Edits[key] = append(bp.Edits[key], edits...)
	}
	stableSortEdits(bp.Edits)
	stableSortEdits(bp.StemEdits)
	bp.Withheld = b.withheld
	return bp
}

// validateBatch normalizes every pair, records each readable source as
// a batch member, and gives each pair its verdict. A pair is planned
// when both paths stay in the workspace, they differ, no earlier pair
// moves its source, no other member lands on its destination, and its
// destination is free once the batch has run: absent from the
// workspace or another member's source.
func validateBatch(ws Workspace, pairs []MovePair) ([]BatchMove, *moveBatch) {
	moves := make([]BatchMove, len(pairs))
	b := &moveBatch{members: map[string]batchMember{}}
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
		case landing[m.Dst] > 1:
			m.Err = ErrDuplicateDestination
		case !vacated && resolves(ws, m.Dst):
			m.Err = DestinationExistsError{Dst: m.Dst}
		default:
			b.members[m.Src] = batchMember{dst: m.Dst, planned: true}
		}
	}
	return moves, b
}

// admit normalizes pr and records its source as a batch member when it
// is readable, counting the member's destination in landing. The
// returned move carries the checks Move runs first, in Move's order: a
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
	key, _, ok := ws.Resolve(m.Src)
	dstOK := workspaceRelative(m.Dst)
	if ok {
		m.Key = key
		member := batchMember{}
		if dstOK {
			member.dst = m.Dst
			landing[m.Dst]++
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
// path and whether its move was planned, and the count of links left
// stale without an edit.
type moveBatch struct {
	members  map[string]batchMember
	withheld int
	post     *linkgraph.WikilinkIndex // postIndex, built on first use
}

// batchMember is one moved file: dst is where the host puts it (empty
// when that is outside the workspace), and planned reports whether
// MoveAll planned its move.
type batchMember struct {
	dst     string
	planned bool
}

// member returns the batch entry for the workspace file p. A resolver
// with no batch (a test driving one pass directly) has no members.
func (r *destResolver) member(p string) (batchMember, bool) {
	if r.batch == nil {
		return batchMember{}, false
	}
	m, ok := r.batch.members[p]
	return m, ok
}

// countStale counts, in the batch, one link that no edit rewrites:
// written as refPath in a file landing at holder, it should name the
// file landing at target. It counts only when that link stops
// resolving, or when either end leaves the workspace.
func (r *destResolver) countStale(holder, refPath, target string) {
	if r.batch == nil {
		return
	}
	if holder == "" || target == "" || linkgraph.ResolveRelTarget(holder, refPath) != target {
		r.batch.withheld++
	}
}
