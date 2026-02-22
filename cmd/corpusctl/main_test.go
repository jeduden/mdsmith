package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/corpus"
)

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()

	if err := run(nil); err == nil || !isUsageError(err) {
		t.Fatalf("expected usage error for empty args, got %v", err)
	}
	if err := run([]string{"unknown"}); err == nil || !isUsageError(err) {
		t.Fatalf("expected usage error for unknown command, got %v", err)
	}
}

func TestRun_FlagValidation(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"build"},
		{"measure"},
		{"qa"},
		{"drift"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			t.Parallel()
			if err := run(args); err == nil || !isUsageError(err) {
				t.Fatalf("expected usage error for %v, got %v", args, err)
			}
		})
	}
}

func TestRun_RoundTripBuildQAAndDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := writeBuildConfig(t, root)

	outDir := filepath.Join(root, "dataset")
	buildArgs := []string{
		"build",
		"-config", configPath,
		"-out", outDir,
		"-cache", filepath.Join(root, "cache"),
	}
	if err := run(buildArgs); err != nil {
		t.Fatalf("run build: %v", err)
	}
	manifestPath := filepath.Join(outDir, "manifest.jsonl")
	reportPath := filepath.Join(outDir, "report.json")
	samplePath := filepath.Join(outDir, "qa-sample.jsonl")
	assertExists(t, manifestPath)
	assertExists(t, reportPath)
	assertExists(t, samplePath)

	sample, err := corpus.ReadQASample(samplePath)
	if err != nil {
		t.Fatalf("ReadQASample: %v", err)
	}
	annotationsPath := filepath.Join(root, "annotations.csv")
	if err := writeAnnotations(sample, annotationsPath); err != nil {
		t.Fatalf("write annotations: %v", err)
	}

	qaReportPath := filepath.Join(outDir, "qa-report.json")
	qaArgs := []string{
		"qa",
		"-sample", samplePath,
		"-annotations", annotationsPath,
		"-out", qaReportPath,
	}
	if err := run(qaArgs); err != nil {
		t.Fatalf("run qa: %v", err)
	}
	assertExists(t, qaReportPath)

	baselinePath := filepath.Join(root, "baseline.json")
	if err := writeBaselineReport(baselinePath); err != nil {
		t.Fatalf("WriteJSON baseline: %v", err)
	}
	driftPath := filepath.Join(outDir, "drift-report.json")
	driftArgs := []string{
		"drift",
		"-baseline", baselinePath,
		"-candidate", reportPath,
		"-out", driftPath,
	}
	if err := run(driftArgs); err != nil {
		t.Fatalf("run drift: %v", err)
	}
	assertExists(t, driftPath)
}

func writeBuildConfig(t *testing.T, root string) string {
	t.Helper()

	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	content := "# API reference\n\nword word word word word word\n"
	if err := os.WriteFile(filepath.Join(sourceRoot, "api.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	configPath := filepath.Join(root, "config.yml")
	config := strings.Join([]string{
		"collected_at: 2026-02-16",
		"min_words: 1",
		"min_chars: 1",
		"license_allowlist:",
		"  - MIT",
		"sources:",
		"  - name: seed",
		"    repository: github.com/acme/seed",
		"    root: " + sourceRoot,
		"    commit_sha: abc123",
		"    license: MIT",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func writeAnnotations(sample []corpus.QASampleRecord, path string) error {
	builder := strings.Builder{}
	builder.WriteString("record_id,actual_category\n")
	for _, row := range sample {
		builder.WriteString(fmt.Sprintf("%s,%s\n", row.RecordID, row.PredictedCategory))
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func writeBaselineReport(path string) error {
	report := corpus.BuildReport{
		DatasetVersion: "v2025-12-15",
		FilesKept:      1,
		Taxonomy: map[corpus.Category]int{
			corpus.CategoryReference: 1,
		},
	}
	return corpus.WriteJSON(path, report)
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}
