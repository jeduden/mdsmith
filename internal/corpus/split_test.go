package corpus

import "testing"

func TestAssignGroupSplits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		size      int
		wantTrain int
		wantDev   int
		wantTest  int
	}{
		{name: "one", size: 1, wantTrain: 0, wantDev: 0, wantTest: 1},
		{name: "two", size: 2, wantTrain: 1, wantDev: 0, wantTest: 1},
		{name: "three", size: 3, wantTrain: 1, wantDev: 1, wantTest: 1},
		{name: "ten", size: 10, wantTrain: 8, wantDev: 1, wantTest: 1},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			group := makeSplitGroup(tt.size)
			assignGroupSplits(group)
			train, dev, test := countSplits(group)
			if train != tt.wantTrain || dev != tt.wantDev || test != tt.wantTest {
				t.Fatalf(
					"counts train/dev/test = %d/%d/%d, want %d/%d/%d",
					train,
					dev,
					test,
					tt.wantTrain,
					tt.wantDev,
					tt.wantTest,
				)
			}
		})
	}
}

func TestAssignSplits_Deterministic(t *testing.T) {
	t.Parallel()

	left := make([]collectedRecord, 0, 12)
	right := make([]collectedRecord, 0, 12)
	for i := 0; i < 8; i++ {
		record := makeCollected(idFor("r", i), CategoryReference, false, nil)
		left = append(left, record)
		right = append(right, record)
	}
	for i := 0; i < 4; i++ {
		record := makeCollected(idFor("h", i), CategoryHowTo, false, nil)
		left = append(left, record)
		right = append(right, record)
	}

	assignSplits(left, 62)
	assignSplits(right, 62)

	leftByID := make(map[string]string, len(left))
	for _, record := range left {
		leftByID[record.RecordID] = record.Split
	}
	for _, record := range right {
		if leftByID[record.RecordID] != record.Split {
			t.Fatalf("split mismatch for %s: %s vs %s", record.RecordID, leftByID[record.RecordID], record.Split)
		}
	}
}

func TestMakeQASample_PerCategoryCap(t *testing.T) {
	t.Parallel()

	records := []ManifestRecord{
		{RecordID: "a1", Category: CategoryReference},
		{RecordID: "a2", Category: CategoryReference},
		{RecordID: "a3", Category: CategoryReference},
		{RecordID: "b1", Category: CategoryHowTo},
		{RecordID: "b2", Category: CategoryHowTo},
		{RecordID: "b3", Category: CategoryHowTo},
	}

	sample := makeQASample(records, 2, 62)
	if len(sample) != 4 {
		t.Fatalf("sample len = %d, want 4", len(sample))
	}
	counts := map[Category]int{}
	for _, row := range sample {
		counts[row.PredictedCategory]++
	}
	if counts[CategoryReference] != 2 || counts[CategoryHowTo] != 2 {
		t.Fatalf("unexpected per-category counts: %+v", counts)
	}
}

func makeSplitGroup(size int) []*collectedRecord {
	group := make([]*collectedRecord, 0, size)
	for i := 0; i < size; i++ {
		record := makeCollected(idFor("g", i), CategoryReference, false, nil)
		group = append(group, &record)
	}
	return group
}

func countSplits(group []*collectedRecord) (int, int, int) {
	train := 0
	dev := 0
	test := 0
	for _, record := range group {
		switch record.Split {
		case SplitTrain:
			train++
		case SplitDev:
			dev++
		case SplitTest:
			test++
		}
	}
	return train, dev, test
}
