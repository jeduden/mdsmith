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
	if report.Coverage != 1 {
		t.Fatalf("Coverage = %f, want 1", report.Coverage)
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

func TestEvaluateQA_ErrorWhenAnnotationsOutsideSample(t *testing.T) {
	t.Parallel()

	_, err := EvaluateQA(
		[]QASampleRecord{{RecordID: "a", PredictedCategory: CategoryReference}},
		[]QAAnnotation{{RecordID: "b", ActualCategory: CategoryReference}},
	)
	if err == nil || !strings.Contains(err.Error(), "not present in sample") {
		t.Fatalf("expected out-of-sample error, got %v", err)
	}
}

func TestEvaluateQA_PartialCoverage(t *testing.T) {
	t.Parallel()

	report, err := EvaluateQA(
		[]QASampleRecord{
			{RecordID: "a", PredictedCategory: CategoryReference},
			{RecordID: "b", PredictedCategory: CategoryOther},
			{RecordID: "c", PredictedCategory: CategoryOther},
		},
		[]QAAnnotation{
			{RecordID: "a", ActualCategory: CategoryReference},
			{RecordID: "b", ActualCategory: CategoryOther},
		},
	)
	if err != nil {
		t.Fatalf("EvaluateQA: %v", err)
	}
	if report.Total != 3 || report.Annotated != 2 {
		t.Fatalf("unexpected totals: total=%d annotated=%d", report.Total, report.Annotated)
	}
	if report.Coverage != (2.0 / 3.0) {
		t.Fatalf("coverage = %f, want %f", report.Coverage, 2.0/3.0)
	}
}

func TestEvaluateQA_DuplicateAnnotationIDs(t *testing.T) {
	t.Parallel()

	_, err := EvaluateQA(
		[]QASampleRecord{{RecordID: "a", PredictedCategory: CategoryReference}},
		[]QAAnnotation{
			{RecordID: "a", ActualCategory: CategoryReference},
			{RecordID: "a", ActualCategory: CategoryOther},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate annotation") {
		t.Fatalf("expected duplicate annotation error, got %v", err)
	}
}

func TestEvaluateQA_EmptyAnnotationsProducesCoverageOnlyReport(t *testing.T) {
	t.Parallel()

	report, err := EvaluateQA(
		[]QASampleRecord{
			{RecordID: "a", PredictedCategory: CategoryReference},
			{RecordID: "b", PredictedCategory: CategoryOther},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("EvaluateQA: %v", err)
	}
	if report.Total != 2 || report.Annotated != 0 {
		t.Fatalf("unexpected totals: total=%d annotated=%d", report.Total, report.Annotated)
	}
	if report.Coverage != 0 {
		t.Fatalf("coverage = %f, want 0", report.Coverage)
	}
	if report.Accuracy != 0 {
		t.Fatalf("accuracy = %f, want 0", report.Accuracy)
	}
	if report.Kappa != nil {
		t.Fatalf("kappa = %v, want nil", *report.Kappa)
	}
	if len(report.Categories) != 0 {
		t.Fatalf("categories = %+v, want empty map", report.Categories)
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

func TestReadQAAnnotationsCSV_IgnoresBlankActualCategory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "annotations.csv")
	content := "record_id,actual_category\na,reference\nb,\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	rows, err := ReadQAAnnotationsCSV(path)
	if err != nil {
		t.Fatalf("ReadQAAnnotationsCSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].RecordID != "a" {
		t.Fatalf("kept record id = %q, want a", rows[0].RecordID)
	}
}

func TestReadQAAnnotationsCSV_EmptyFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "annotations.csv")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	rows, err := ReadQAAnnotationsCSV(path)
	if err != nil {
		t.Fatalf("ReadQAAnnotationsCSV: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("len(rows) = %d, want 0", len(rows))
	}
}

func TestReadQAAnnotationsCSV_IgnoresBlankRows(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "annotations.csv")
	content := "record_id,actual_category\n,\n\na,reference\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	rows, err := ReadQAAnnotationsCSV(path)
	if err != nil {
		t.Fatalf("ReadQAAnnotationsCSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].RecordID != "a" {
		t.Fatalf("kept record id = %q, want a", rows[0].RecordID)
	}
}

func TestWriteQAAnnotationTemplateCSV(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "template.csv")
	sample := []QASampleRecord{
		{RecordID: "a", PredictedCategory: CategoryReference},
		{RecordID: "b", PredictedCategory: CategoryOther},
	}
	existing := []QAAnnotation{{RecordID: "a", ActualCategory: CategoryReference}}

	stats, err := WriteQAAnnotationTemplateCSV(path, sample, existing)
	if err != nil {
		t.Fatalf("WriteQAAnnotationTemplateCSV: %v", err)
	}
	if stats.Total != 2 || stats.Preserved != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read template file: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "record_id,actual_category\n") {
		t.Fatalf("missing header: %q", got)
	}
	if !strings.Contains(got, "a,reference\n") {
		t.Fatalf("missing preserved row: %q", got)
	}
	if !strings.Contains(got, "b,\n") {
		t.Fatalf("missing blank row: %q", got)
	}
}
