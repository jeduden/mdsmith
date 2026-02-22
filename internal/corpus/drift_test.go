package corpus

import "testing"

func TestCompareReports(t *testing.T) {
	t.Parallel()

	baseline := BuildReport{
		DatasetVersion: "v2025-12-15",
		FilesKept:      100,
		Taxonomy: map[Category]int{
			CategoryReference: 70,
			CategoryOther:     30,
		},
		Metrics: MetricSummary{AvgWords: 120, AvgChars: 800},
	}
	candidate := BuildReport{
		DatasetVersion: "v2026-02-16",
		FilesKept:      112,
		Taxonomy: map[Category]int{
			CategoryReference: 75,
			CategoryOther:     37,
		},
		Metrics: MetricSummary{AvgWords: 130, AvgChars: 820},
	}

	drift := CompareReports(baseline, candidate)
	if drift.FilesKeptDelta != 12 {
		t.Fatalf("FilesKeptDelta = %d, want 12", drift.FilesKeptDelta)
	}
	if drift.TaxonomyDeltas[CategoryReference] != 5 {
		t.Fatalf("reference taxonomy delta = %d, want 5", drift.TaxonomyDeltas[CategoryReference])
	}
	if drift.TaxonomyDeltas[CategoryOther] != 7 {
		t.Fatalf("other taxonomy delta = %d, want 7", drift.TaxonomyDeltas[CategoryOther])
	}
	if drift.MetricDeltas.AvgWords != 10 || drift.MetricDeltas.AvgChars != 20 {
		t.Fatalf("unexpected metric deltas: %+v", drift.MetricDeltas)
	}
}
