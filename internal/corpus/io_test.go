package corpus

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteManifest_WritesJSONL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	records := []Record{{
		RecordID: "a",
		Source:   "seed",
		Path:     "docs/a.md",
		Category: CategoryReference,
		Words:    10,
		Chars:    50,
	}}
	if err := WriteManifest(path, records); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("expected one manifest line")
	}
	var got Record
	if err := json.Unmarshal([]byte(scanner.Text()), &got); err != nil {
		t.Fatalf("unmarshal manifest row: %v", err)
	}
	if got.RecordID != "a" || got.Path != "docs/a.md" {
		t.Fatalf("unexpected manifest row: %+v", got)
	}
}

func TestWriteJSONAndReadBuildReport(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.json")
	want := BuildReport{DatasetVersion: "v2026-02-16", FilesKept: 12}
	if err := WriteJSON(path, want); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got, err := ReadBuildReport(path)
	if err != nil {
		t.Fatalf("ReadBuildReport: %v", err)
	}
	if got.DatasetVersion != want.DatasetVersion || got.FilesKept != want.FilesKept {
		t.Fatalf("unexpected report: %+v", got)
	}
}

func TestReadBuildReport_InvalidJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := ReadBuildReport(path)
	if err == nil || !strings.Contains(err.Error(), "parse build report json") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
