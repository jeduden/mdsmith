package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/corpus"
)

func TestRunMeasure_ValidatesFlags(t *testing.T) {
	t.Parallel()

	if err := runMeasure(nil); err == nil || !isUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestRunMeasure_WritesReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.jsonl")
	records := []corpus.Record{
		{RecordID: "a", Category: corpus.CategoryReference, Words: 10, Chars: 100},
		{RecordID: "b", Category: corpus.CategoryOther, Words: 20, Chars: 200},
	}
	if err := corpus.WriteManifest(manifestPath, records); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	outPath := filepath.Join(dir, "measure.json")
	if err := runMeasure([]string{"-corpus", dir, "-out", outPath}); err != nil {
		t.Fatalf("runMeasure: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output report: %v", err)
	}
}

func TestRunMeasure_InvalidManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.jsonl"), []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	err := runMeasure([]string{"-corpus", dir, "-out", filepath.Join(dir, "measure.json")})
	if err == nil || !strings.Contains(err.Error(), "parse manifest row") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
