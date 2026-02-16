package corpus

import "testing"

func TestCompareReports(t *testing.T) {
	t.Parallel()

	baseline := BuildReport{
		DatasetVersion: "v1",
		FilesKept:      10,
		ReadmeShare:    0.3,
		CategoryCounts: map[Category]int{
			CategoryReference:   6,
			CategoryProjectDocs: 4,
		},
	}
	candidate := BuildReport{
		DatasetVersion: "v2",
		FilesKept:      12,
		ReadmeShare:    0.2,
		CategoryCounts: map[Category]int{
			CategoryReference:   5,
			CategoryProjectDocs: 4,
			CategoryHowTo:       3,
		},
	}

	drift := CompareReports(baseline, candidate)
	if got, want := drift.DeltaTotal, 2; got != want {
		t.Fatalf("DeltaTotal = %d, want %d", got, want)
	}
	if got, want := drift.ReadmeShareDelta, -0.1; got-want > 0.000001 || want-got > 0.000001 {
		t.Fatalf("ReadmeShareDelta = %f, want %f", got, want)
	}
	if drift.ByCategory[CategoryHowTo].CandidateCount != 3 {
		t.Fatalf("CategoryHowTo candidate count = %d, want 3", drift.ByCategory[CategoryHowTo].CandidateCount)
	}
}
