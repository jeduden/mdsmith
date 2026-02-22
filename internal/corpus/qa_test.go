package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateQA_ComputesMetricsAndKappa(t *testing.T) {
	t.Parallel()

	sample := []QASampleRecord{
		{RecordID: "a", PredictedCategory: CategoryReference},
		{RecordID: "b", PredictedCategory: CategoryReference},
		{RecordID: "c", PredictedCategory: CategoryOther},
	}
	annotations := []QAAnnotation{
		{RecordID: "a", ActualCategory: CategoryReference},
		{RecordID: "b", ActualCategory: CategoryOther},
		{RecordID: "c", ActualCategory: CategoryOther},
	}

	report, err := EvaluateQA(sample, annotations)
	if err != nil {
		t.Fatalf("EvaluateQA: %v", err)
	}
	if report.Total != 3 {
		t.Fatalf("Total = %d, want 3", report.Total)
	}
	if report.Annotated != 3 {
		t.Fatalf("Annotated = %d, want 3", report.Annotated)
	}
	if report.Accuracy <= 0 || report.Accuracy >= 1 {
		t.Fatalf("Accuracy = %f, want value in (0,1)", report.Accuracy)
	}
	if report.Kappa == nil {
		t.Fatal("Kappa should be computed")
	}
	if _, ok := report.Categories[CategoryReference]; !ok {
		t.Fatal("missing reference metrics")
	}
	if _, ok := report.Categories[CategoryOther]; !ok {
		t.Fatal("missing other metrics")
	}
}

func TestEvaluateQA_ErrorWhenNoOverlap(t *testing.T) {
	t.Parallel()

	_, err := EvaluateQA(
		[]QASampleRecord{{RecordID: "a", PredictedCategory: CategoryReference}},
		[]QAAnnotation{{RecordID: "b", ActualCategory: CategoryReference}},
	)
	if err == nil || !strings.Contains(err.Error(), "no overlapping record ids") {
		t.Fatalf("expected overlap error, got %v", err)
	}
}

func TestQASampleReadWriteRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "qa-sample.jsonl")
	rows := []QASampleRecord{{RecordID: "a", PredictedCategory: CategoryReference, Source: "seed", Path: "docs/a.md"}}
	if err := WriteQASample(path, rows); err != nil {
		t.Fatalf("WriteQASample: %v", err)
	}
	got, err := ReadQASample(path)
	if err != nil {
		t.Fatalf("ReadQASample: %v", err)
	}
	if len(got) != 1 || got[0].RecordID != "a" {
		t.Fatalf("unexpected qa sample: %+v", got)
	}
}

func TestReadQAAnnotationsCSV(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "annotations.csv")
	if err := os.WriteFile(path, []byte("record_id,actual_category\na,reference\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	rows, err := ReadQAAnnotationsCSV(path)
	if err != nil {
		t.Fatalf("ReadQAAnnotationsCSV: %v", err)
	}
	if len(rows) != 1 || rows[0].ActualCategory != CategoryReference {
		t.Fatalf("unexpected annotation rows: %+v", rows)
	}
}

func TestReadQAAnnotationsCSV_ErrorPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "annotations.csv")
	if err := os.WriteFile(path, []byte("record_id,actual_category\na,invalid\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	_, err := ReadQAAnnotationsCSV(path)
	if err == nil || !strings.Contains(err.Error(), "unknown category") {
		t.Fatalf("expected category error, got %v", err)
	}
}
