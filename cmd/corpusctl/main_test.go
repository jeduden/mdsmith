package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/corpus"
)

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()

	if err := run(nil); err == nil {
		t.Fatal("expected usage error for empty args")
	}
	if err := run([]string{"unknown"}); err == nil {
		t.Fatal("expected usage error for unknown command")
	}
}

func TestRun_FlagValidation(t *testing.T) {
	t.Parallel()

	if err := run([]string{"build"}); err == nil {
		t.Fatal("expected build flag error")
	}
	if err := run([]string{"qa"}); err == nil {
		t.Fatal("expected qa flag error")
	}
	if err := run([]string{"drift"}); err == nil {
		t.Fatal("expected drift flag error")
	}
}

func TestRunBuild_WritesOutputs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# Title\n\nword word\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	configPath := filepath.Join(root, "config.yml")
	config := strings.TrimSpace(`
collected_at: 2026-02-16
min_words: 1
min_chars: 1
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: source
    commit_sha: abc123
    license: MIT
`) + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	outDir := filepath.Join(root, "out")
	if err := run([]string{"build", "-config", configPath, "-out", outDir}); err != nil {
		t.Fatalf("run build: %v", err)
	}
	assertExists(t, filepath.Join(outDir, "manifest.jsonl"))
	assertExists(t, filepath.Join(outDir, "report.json"))
	assertExists(t, filepath.Join(outDir, "qa-sample.jsonl"))
}

func TestRunQA_WritesReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	samplePath := filepath.Join(dir, "sample.jsonl")
	annotationsPath := filepath.Join(dir, "annotations.csv")
	outPath := filepath.Join(dir, "qa-report.json")

	err := corpus.WriteQASample(samplePath, []corpus.QASampleRecord{{
		RecordID:          "a",
		PredictedCategory: corpus.CategoryReference,
	}})
	if err != nil {
		t.Fatalf("WriteQASample: %v", err)
	}
	if err := os.WriteFile(annotationsPath, []byte("record_id,actual_category\na,reference\n"), 0o644); err != nil {
		t.Fatalf("write annotations: %v", err)
	}

	if err := run([]string{"qa", "-sample", samplePath, "-annotations", annotationsPath, "-out", outPath}); err != nil {
		t.Fatalf("run qa: %v", err)
	}
	assertExists(t, outPath)
}

func TestRunDrift_WritesReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	candidatePath := filepath.Join(dir, "candidate.json")
	outPath := filepath.Join(dir, "drift.json")

	if err := corpus.WriteJSON(baselinePath, corpus.BuildReport{DatasetVersion: "v1", FilesKept: 10}); err != nil {
		t.Fatalf("WriteJSON baseline: %v", err)
	}
	if err := corpus.WriteJSON(candidatePath, corpus.BuildReport{DatasetVersion: "v2", FilesKept: 12}); err != nil {
		t.Fatalf("WriteJSON candidate: %v", err)
	}

	args := []string{
		"drift",
		"-baseline", baselinePath,
		"-candidate", candidatePath,
		"-out", outPath,
	}
	if err := run(args); err != nil {
		t.Fatalf("run drift: %v", err)
	}
	assertExists(t, outPath)
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}
