package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	want := []ManifestRecord{{RecordID: "a", Category: CategoryReference, Path: "docs/a.md"}}
	if err := WriteManifest(path, want); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if len(got) != 1 || got[0].RecordID != "a" {
		t.Fatalf("unexpected manifest round-trip: %+v", got)
	}
}

func TestReadManifest_InvalidJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	if err := os.WriteFile(path, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := ReadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "parse manifest line") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestQASampleRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "qa-sample.jsonl")
	want := []QASampleRecord{{RecordID: "a", PredictedCategory: CategoryHowTo}}
	if err := WriteQASample(path, want); err != nil {
		t.Fatalf("WriteQASample: %v", err)
	}
	got, err := ReadQASample(path)
	if err != nil {
		t.Fatalf("ReadQASample: %v", err)
	}
	if len(got) != 1 || got[0].PredictedCategory != CategoryHowTo {
		t.Fatalf("unexpected qa sample round-trip: %+v", got)
	}
}

func TestReadQASample_InvalidJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "qa-sample.jsonl")
	if err := os.WriteFile(path, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := ReadQASample(path)
	if err == nil || !strings.Contains(err.Error(), "parse qa sample line") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestReadQAAnnotationsCSV(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "annotations.csv")
	content := "record_id,actual_category\na,reference\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rows, err := ReadQAAnnotationsCSV(path)
	if err != nil {
		t.Fatalf("ReadQAAnnotationsCSV: %v", err)
	}
	if len(rows) != 1 || rows[0].ActualCategory != CategoryReference {
		t.Fatalf("unexpected annotations rows: %+v", rows)
	}
}

func TestReadQAAnnotationsCSV_InvalidRow(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "annotations.csv")
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := ReadQAAnnotationsCSV(path)
	if err == nil || !strings.Contains(err.Error(), "record_id,actual_category") {
		t.Fatalf("expected row-shape error, got %v", err)
	}
}

func TestWriteJSONAndReadBuildReport(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.json")
	want := BuildReport{DatasetVersion: "v1", FilesKept: 3}
	if err := WriteJSON(path, want); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got, err := ReadBuildReport(path)
	if err != nil {
		t.Fatalf("ReadBuildReport: %v", err)
	}
	if got.DatasetVersion != "v1" || got.FilesKept != 3 {
		t.Fatalf("unexpected report round-trip: %+v", got)
	}
}
