package main

import (
	"testing"

	buildexec "github.com/jeduden/mdsmith/internal/build"
	"github.com/jeduden/mdsmith/internal/rename"
)

// These pin the allocation cost of five CLI output sorts that used
// sort.Slice / sort.SliceStable, which drive reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md, already fixed the same
// way elsewhere in this codebase (astutil.sortSectionHeadings,
// mdsmith.sortDirEntries). Each sorts a command-scoped result list
// once per CLI invocation, not per workspace file, so the win is
// small but free and matches the project's own established
// convention. slices.SortFunc on the concrete result type sorts with
// no reflection.

func TestSortRenameSummaries_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	summaries := []renameSummary{
		{File: "zeta.md", Edits: 1},
		{File: "alpha.md", Edits: 2},
		{File: "mu.md", Edits: 3},
	}
	sortRenameSummaries(summaries)
	if summaries[0].File != "alpha.md" || summaries[2].File != "zeta.md" {
		t.Fatalf("sortRenameSummaries did not sort: %v", summaries)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortRenameSummaries(summaries)
	})
	t.Logf("sortRenameSummaries allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortRenameSummaries allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

func TestSortEditsByCharacterDesc_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	es := []rename.Edit{
		{Range: rename.Range{Start: rename.Position{Character: 3}}},
		{Range: rename.Range{Start: rename.Position{Character: 9}}},
		{Range: rename.Range{Start: rename.Position{Character: 1}}},
	}
	sortEditsByCharacterDesc(es)
	if es[0].Range.Start.Character != 9 || es[2].Range.Start.Character != 1 {
		t.Fatalf("sortEditsByCharacterDesc did not sort descending: %v", es)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortEditsByCharacterDesc(es)
	})
	t.Logf("sortEditsByCharacterDesc allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortEditsByCharacterDesc allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

func TestSortDepRecords_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	recs := []depRecord{
		{Line: 5, Target: "b.md"},
		{Line: 2, Target: "z.md"},
		{Line: 2, Target: "a.md"},
	}
	sortDepRecords(recs)
	if recs[0].Line != 2 || recs[0].Target != "a.md" || recs[2].Line != 5 {
		t.Fatalf("sortDepRecords did not sort: %v", recs)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortDepRecords(recs)
	})
	t.Logf("sortDepRecords allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortDepRecords allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

func TestSortBacklinkRecords_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	records := []backlinkRecord{
		{Source: "z.md", Line: 1},
		{Source: "a.md", Line: 5},
		{Source: "a.md", Line: 2},
	}
	sortBacklinkRecords(records)
	if records[0].Source != "a.md" || records[0].Line != 2 || records[2].Source != "z.md" {
		t.Fatalf("sortBacklinkRecords did not sort: %v", records)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortBacklinkRecords(records)
	})
	t.Logf("sortBacklinkRecords allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortBacklinkRecords allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

func TestSortBuildTargets_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	targets := []buildTarget{
		{file: "z.md", line: 1, target: buildexec.Target{}},
		{file: "a.md", line: 5, target: buildexec.Target{}},
		{file: "a.md", line: 2, target: buildexec.Target{}},
	}
	sortBuildTargets(targets)
	if targets[0].file != "a.md" || targets[0].line != 2 || targets[2].file != "z.md" {
		t.Fatalf("sortBuildTargets did not sort: %v", targets)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortBuildTargets(targets)
	})
	t.Logf("sortBuildTargets allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortBuildTargets allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
