package corpus

import "testing"

func TestSplit_Deterministic(t *testing.T) {
	t.Parallel()

	records := []Record{
		{RecordID: "a"},
		{RecordID: "b"},
		{RecordID: "c"},
		{RecordID: "d"},
		{RecordID: "e"},
	}

	trainA, testA := Split(records, 0.4)
	trainB, testB := Split(records, 0.4)

	if len(trainA) != len(trainB) || len(testA) != len(testB) {
		t.Fatalf("non-deterministic split sizes: %d/%d vs %d/%d", len(trainA), len(testA), len(trainB), len(testB))
	}
	for i := range trainA {
		if trainA[i].RecordID != trainB[i].RecordID || trainA[i].Split != SplitTrain {
			t.Fatalf("train mismatch at %d: %+v vs %+v", i, trainA[i], trainB[i])
		}
	}
	for i := range testA {
		if testA[i].RecordID != testB[i].RecordID || testA[i].Split != SplitTest {
			t.Fatalf("test mismatch at %d: %+v vs %+v", i, testA[i], testB[i])
		}
	}
}

func TestSplit_DefaultFraction(t *testing.T) {
	t.Parallel()

	records := []Record{{RecordID: "a"}, {RecordID: "b"}, {RecordID: "c"}}
	_, test := Split(records, 0)
	if len(test) == 0 || len(test) >= len(records) {
		t.Fatalf("unexpected default-fraction test size: %d", len(test))
	}
}
