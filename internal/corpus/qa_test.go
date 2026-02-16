package corpus

import "testing"

func TestEvaluateQA(t *testing.T) {
	t.Parallel()

	sample := []QASampleRecord{
		{RecordID: "a", PredictedCategory: CategoryReference},
		{RecordID: "b", PredictedCategory: CategoryHowTo},
		{RecordID: "c", PredictedCategory: CategoryHowTo},
	}
	annotations := []QAAnnotation{
		{RecordID: "a", ActualCategory: CategoryReference},
		{RecordID: "b", ActualCategory: CategoryTutorial},
		{RecordID: "c", ActualCategory: CategoryHowTo},
	}

	report, err := EvaluateQA(sample, annotations)
	if err != nil {
		t.Fatalf("EvaluateQA: %v", err)
	}
	if got, want := report.Total, 3; got != want {
		t.Fatalf("Total = %d, want %d", got, want)
	}
	if got, want := report.Agreement, 2.0/3.0; got != want {
		t.Fatalf("Agreement = %f, want %f", got, want)
	}
	if len(report.ConfusionCases) != 1 {
		t.Fatalf("ConfusionCases len = %d, want 1", len(report.ConfusionCases))
	}

	howto := report.PerCategory[CategoryHowTo]
	if howto.Precision >= 1.0 {
		t.Fatalf("expected how-to precision < 1, got %f", howto.Precision)
	}
}
