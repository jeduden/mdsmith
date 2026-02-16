package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPipeline(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCorpus(t, root)
	cfg := testBuildConfig(root)

	result, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	assertReport(t, result)
	assertManifestRecords(t, result.Manifest)
	assertDeterministicOutput(t, cfg, result.Manifest)
}

func seedCorpus(t *testing.T, root string) {
	t.Helper()
	writeMarkdown(t, filepath.Join(root, "AGENTS.md"), headingWithWords("Agent", 40))
	writeMarkdown(t, filepath.Join(root, "README.md"), headingWithWords("Readme", 40))
	writeMarkdown(t, filepath.Join(root, "docs/tutorial/install.md"), headingWithWords("Tutorial", 40))
	writeMarkdown(t, filepath.Join(root, "docs/reference/rule.md"), headingWithWords("Reference", 40))
	writeMarkdown(t, filepath.Join(root, "docs/reference/rule-copy.md"), headingWithWords("Reference", 40))
	writeMarkdown(t, filepath.Join(root, "generated/auto.md"), "# Generated\n\nCode generated. Do not edit.\n")
	writeMarkdown(t, filepath.Join(root, "docs/short.md"), "# Short\n\nsmall\n")
}

func testBuildConfig(root string) BuildConfig {
	return BuildConfig{
		DatasetVersion:         "v2026-02-16",
		CollectedAt:            "2026-02-16",
		Seed:                   62,
		MinWords:               10,
		MinChars:               40,
		NearDuplicateThreshold: 0.95,
		MaxReadmeShare:         0.5,
		QASamplePerCategory:    2,
		LicenseAllowlist:       []string{"MIT"},
		Policy:                 QualityPolicy{},
		Sources: []SourceConfig{
			{
				Name:       "seed",
				Repository: "github.com/acme/seed",
				Root:       root,
				CommitSHA:  "abc123",
				License:    "MIT",
				Include:    []string{"*.md", "**/*.md"},
				Quality: SourceQuality{
					Stars:            10,
					RecentCommits90D: 5,
					HasCI:            true,
				},
			},
		},
	}
}

func assertReport(t *testing.T, result BuildOutput) {
	t.Helper()
	if got, want := result.Report.FilteredGenerated, 1; got != want {
		t.Fatalf("FilteredGenerated = %d, want %d", got, want)
	}
	if got, want := result.Report.FilteredLowSignal, 1; got != want {
		t.Fatalf("FilteredLowSignal = %d, want %d", got, want)
	}
	if got, want := result.Report.DroppedExactDupes, 1; got != want {
		t.Fatalf("DroppedExactDupes = %d, want %d", got, want)
	}
	if got, want := len(result.Manifest), 4; got != want {
		t.Fatalf("manifest len = %d, want %d", got, want)
	}
	if len(result.QASample) == 0 {
		t.Fatal("qa sample should not be empty")
	}
}

func assertManifestRecords(t *testing.T, records []ManifestRecord) {
	t.Helper()
	for _, record := range records {
		if record.Repository == "" || record.Path == "" || record.CommitSHA == "" || record.License == "" {
			t.Fatalf("record missing provenance metadata: %+v", record)
		}
		if record.Split == "" {
			t.Fatalf("record missing split assignment: %+v", record)
		}
	}
}

func assertDeterministicOutput(t *testing.T, cfg BuildConfig, manifest []ManifestRecord) {
	t.Helper()
	repeat, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build second run: %v", err)
	}
	if len(repeat.Manifest) != len(manifest) {
		t.Fatalf("non-deterministic manifest size: %d vs %d", len(repeat.Manifest), len(manifest))
	}
	for i := range manifest {
		a := manifest[i]
		b := repeat.Manifest[i]
		if a.RecordID != b.RecordID || a.Split != b.Split {
			t.Fatalf("non-deterministic output at %d: %+v vs %+v", i, a, b)
		}
	}
}

func writeMarkdown(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func headingWithWords(title string, words int) string {
	text := "# " + title + "\n\n"
	for i := 0; i < words; i++ {
		text += "token "
	}
	return text + "\n"
}
