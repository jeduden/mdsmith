package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/refactor"
)

// This file holds the two phases behind applyPlan, which makes a
// multi-file refactor all-or-nothing:
//
//  1. computePlanWrites reads every file and splices its edits in
//     memory. Any failure aborts before a byte is written. A dry run
//     stops after this phase, so it reports the same failures.
//  2. commitPlan writes the results. It stages every file's new bytes
//     to a temp sibling, then renames each temp over its file, then runs
//     the FileOp. A failure while staging leaves every file untouched; a
//     failed rename or FileOp restores the files already replaced.

// planWrite is one file's pending rewrite: computed in applyPlan's
// first phase, written in its second.
type planWrite struct {
	rel   string // workspace-relative key, for reports
	abs   string // on-disk path the new bytes replace
	orig  []byte // bytes read in phase one, restored on rollback
	data  []byte // bytes after splicing the edits
	edits int    // number of edits spliced, for the report
}

// computePlanWrites is applyPlan's first phase. It reads every file
// that has edits and splices them in memory, in path order, writing
// nothing. The first read or splice failure is reported on stderr and
// returns exit 2, so a failing plan changes no file.
func computePlanWrites(ws cliRenameWorkspace, edits map[string][]refactor.Edit) ([]planWrite, int) {
	rels := make([]string, 0, len(edits))
	for rel, es := range edits {
		if len(es) > 0 {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	writes := make([]planWrite, 0, len(rels))
	for _, rel := range rels {
		pw, err := computePlanWrite(ws, rel, edits[rel])
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return nil, 2
		}
		writes = append(writes, pw)
	}
	return writes, 0
}

// computePlanWrite reads rel and splices edits into it in memory. The
// on-disk target is rel's discovered path, or rel joined to the
// workspace root when discovery did not list it.
func computePlanWrite(ws cliRenameWorkspace, rel string, edits []refactor.Edit) (planWrite, error) {
	_, src, ok := ws.Resolve(rel)
	if !ok {
		return planWrite{}, fmt.Errorf("cannot read %q to apply edits", rel)
	}
	out, err := refactor.ApplyEdits(src, edits)
	if err != nil {
		return planWrite{}, fmt.Errorf("%s: %w", rel, err)
	}
	abs, ok := ws.relToAbs[rel]
	if !ok {
		abs = filepath.Join(ws.rootDir, filepath.FromSlash(rel))
	}
	return planWrite{rel: rel, abs: abs, orig: src, data: out, edits: len(edits)}, nil
}

// commitFailure describes a failed write phase: the cause, how many
// replaced files the rollback restored, and each file it could not
// restore. A file in unrestored keeps its rewritten bytes.
type commitFailure struct {
	cause      error
	restored   int
	unrestored []restoreFailure
}

// restoreFailure names a file the rollback could not restore, and why.
type restoreFailure struct {
	rel string
	err error
}

// commitPlan is applyPlan's second phase. It stages every write first,
// so the likely failures (a full disk, a read-only directory) surface
// before any file is touched. Then it renames each staged temp over its
// file, in order, and last runs op: the file moves only once every
// reference edit is in place. When a rename or op fails, the files
// already replaced get their original bytes back. It returns nil on
// success.
func commitPlan(rootDir string, writes []planWrite, op *refactor.FileOp) *commitFailure {
	staged, err := stageWrites(writes)
	if err != nil {
		return &commitFailure{cause: err}
	}
	for i, pw := range writes {
		if err := replaceWithStaged(staged[i], pw.abs); err != nil {
			removeStaged(staged[i+1:])
			return rollbackWrites(writes[:i], fmt.Errorf("writing %s: %w", pw.rel, err))
		}
	}
	if op != nil {
		if err := op.Execute(rootDir); err != nil {
			return rollbackWrites(writes, err)
		}
	}
	return nil
}

// stageWrites stages each write's new bytes beside its file (stageFile)
// and returns the temp names, index-aligned with writes. On the first
// failure it removes the temps staged so far and returns an error
// naming the file.
func stageWrites(writes []planWrite) ([]string, error) {
	staged := make([]string, 0, len(writes))
	for _, pw := range writes {
		tmp, err := stageFile(pw.abs, pw.data)
		if err != nil {
			removeStaged(staged)
			return nil, fmt.Errorf("writing %s: %w", pw.rel, err)
		}
		staged = append(staged, tmp)
	}
	return staged, nil
}

// removeStaged deletes staged temp files, best effort: a leftover temp
// is litter beside a file, never a change to it.
func removeStaged(tmps []string) {
	for _, tmp := range tmps {
		_ = os.Remove(tmp)
	}
}

// rollbackWrites writes back the original bytes of every file in done
// (each already replaced) and returns the failure for cause, counting
// the files restored and listing the ones that could not be.
func rollbackWrites(done []planWrite, cause error) *commitFailure {
	f := &commitFailure{cause: cause}
	for _, pw := range done {
		if err := writeFilePreservingMode(pw.abs, pw.orig); err != nil {
			f.unrestored = append(f.unrestored, restoreFailure{rel: pw.rel, err: err})
			continue
		}
		f.restored++
	}
	return f
}

// message renders the failure for stderr: the cause, then a last line
// with the workspace state. That is one of: no file changed, every
// replaced file restored, or each file the rollback missed, which
// keeps its rewritten bytes.
func (f *commitFailure) message() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mdsmith: %v\n", f.cause)
	switch {
	case len(f.unrestored) > 0:
		rels := make([]string, 0, len(f.unrestored))
		for _, u := range f.unrestored {
			fmt.Fprintf(&b, "mdsmith: restoring %s: %v\n", u.rel, u.err)
			rels = append(rels, u.rel)
		}
		fmt.Fprintf(&b, "mdsmith: %d file(s) keep the rewritten content: %s\n",
			len(rels), strings.Join(rels, ", "))
	case f.restored > 0:
		fmt.Fprintf(&b, "mdsmith: restored %d file(s) to their original content\n", f.restored)
	default:
		b.WriteString("mdsmith: no file was changed\n")
	}
	return b.String()
}
