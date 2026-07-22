package corpus

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/bytelimit"
)

func TestCollect_HappyPath(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	apiPath := filepath.Join(root, "api.md")
	apiContent := []byte("# API Reference\n\nword word word word word word\n")
	if err := os.WriteFile(apiPath, apiContent, 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tiny.md"), []byte("small\n"), 0o644); err != nil {
		t.Fatalf("write tiny markdown: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not markdown"), 0o644); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         5,
		MinChars:         10,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       root,
			CommitSHA:  "abc123",
			License:    "MIT",
		}},
	}

	records, err := Collect(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Source != "seed" {
		t.Fatalf("Source = %q, want seed", record.Source)
	}
	if record.Path != "api.md" {
		t.Fatalf("Path = %q, want api.md", record.Path)
	}
	if record.Words < 5 || record.Chars < 10 {
		t.Fatalf("unexpected counts: words=%d chars=%d", record.Words, record.Chars)
	}
	if record.RawContent == "" {
		t.Fatal("RawContent should be populated")
	}
}

func TestCollect_SkipsDisallowedLicense(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Doc\n\nword word word word\n"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         1,
		MinChars:         1,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       root,
			CommitSHA:  "abc123",
			License:    "Apache-2.0",
		}},
	}

	records, err := Collect(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("record count = %d, want 0", len(records))
	}
}

func TestIsGenerated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		// Should match generated/vendored paths.
		{"vendor/pkg/doc.md", true},
		{"node_modules/lib/README.md", true},
		{"dist/output.md", true},
		{"build/report.md", true},
		{"generated/api.md", true},
		{"gen/schema.md", true},
		{"deep/nested/vendor/pkg/doc.md", true},
		{"src/node_modules/lib/README.md", true},

		// Root-level directories (the leading "/" normalization catches these).
		{"vendor/doc.md", true},
		{"node_modules/doc.md", true},

		// Should NOT match: tokens that appear as substrings of other names.
		{"docs/general.md", false},
		{"src/adventure.md", false},
		{"src/api.md", false},
		{"README.md", false},
		{"docs/guide.md", false},
	}

	for _, tt := range tests {
		if got := isGenerated(tt.path); got != tt.want {
			t.Errorf("isGenerated(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCollect_SkipsGeneratedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create a normal markdown file that should be collected.
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	content := []byte("# Guide\n\nword word word word word word\n")
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), content, 0o644); err != nil {
		t.Fatalf("write guide.md: %v", err)
	}

	// Create markdown files in generated/vendored directories that should be skipped.
	for _, dir := range []string{"vendor/pkg", "node_modules/lib", "dist", "build", "generated", "gen"} {
		dirPath := filepath.Join(root, dir)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dirPath, "doc.md"), content, 0o644); err != nil {
			t.Fatalf("write %s/doc.md: %v", dir, err)
		}
	}

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         1,
		MinChars:         1,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       root,
			CommitSHA:  "abc123",
			License:    "MIT",
		}},
	}

	records, err := Collect(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(records) != 1 {
		paths := make([]string, len(records))
		for i, r := range records {
			paths[i] = r.Path
		}
		t.Fatalf("record count = %d, want 1 (only docs/guide.md); got paths %v", len(records), paths)
	}
	if records[0].Path != "docs/guide.md" {
		t.Fatalf("Path = %q, want docs/guide.md", records[0].Path)
	}
}

func TestCollect_ErrorPath(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         1,
		MinChars:         1,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       filepath.Join(t.TempDir(), "missing"),
			CommitSHA:  "abc123",
			License:    "MIT",
		}},
	}

	_, err := Collect(cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected resolve error")
	}
}

// TestCollect_OversizedFile_SkippedNotFatal guards against an unbounded
// os.ReadFile on a corpus source: collectFile ingests markdown from
// cloned third-party repositories, which are untrusted input
// (docs/development/high-performance-go.md — "os.ReadFile on huge
// inputs: one giant alloc, all resident"). A file over the shared
// bytelimit.DefaultMaxInputBytes cap must be skipped rather than read
// into memory in full — and, since a real source repository can contain
// one large file among many good ones (a big CHANGELOG, a vendored
// spec), skipping it must not abort collection of the rest of that
// source, or of sources collected *earlier* in the same run (Collect's
// loop over cfg.Sources returns on the first error, discarding every
// record gathered so far — so this uses two sources, not one, to prove
// the first source's record survives a later source's oversized file).
func TestCollect_OversizedFile_SkippedNotFatal(t *testing.T) {
	t.Parallel()

	const prose = "# Title\n\nword word word word word word\n"

	goodRoot := filepath.Join(t.TempDir(), "good")
	mustMkdirAll(t, goodRoot)
	mustWriteFile(t, filepath.Join(goodRoot, "early.md"), []byte(prose))

	mixedRoot := filepath.Join(t.TempDir(), "mixed")
	mustMkdirAll(t, mixedRoot)
	oversized := bytes.Repeat([]byte("a "), int(bytelimit.DefaultMaxInputBytes)/2+1)
	mustWriteFile(t, filepath.Join(mixedRoot, "huge.md"), oversized)
	mustWriteFile(t, filepath.Join(mixedRoot, "normal.md"), []byte(prose))

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         1,
		MinChars:         1,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{
			{
				Name:       "early",
				Repository: "github.com/acme/early",
				Root:       goodRoot,
				CommitSHA:  "abc123",
				License:    "MIT",
			},
			{
				Name:       "mixed",
				Repository: "github.com/acme/mixed",
				Root:       mixedRoot,
				CommitSHA:  "def456",
				License:    "MIT",
			},
		},
	}

	records, err := Collect(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Collect: unexpected error, oversized file should be skipped: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2 (early.md from the first source, "+
			"normal.md from the second; huge.md must be skipped)", len(records))
	}
	paths := make([]string, len(records))
	for i, r := range records {
		paths[i] = r.Source + "/" + r.Path
	}
	if paths[0] != "early/early.md" || paths[1] != "mixed/normal.md" {
		t.Fatalf("records = %v, want [early/early.md mixed/normal.md]", paths)
	}
}

// mustMkdirAll creates dir and all parents, failing the test on error.
func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// mustWriteFile writes content to path, failing the test on error.
func mustWriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestCollectFile_StatError_SkippedNotFatal covers collectFile's os.Stat
// error branch directly: a file that vanishes between WalkDir listing it
// and the Stat call inside collectFile (or any other stat failure) must
// be skipped, not treated as fatal — the same reasoning as the oversized-
// file case above.
func TestCollectFile_StatError_SkippedNotFatal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	missing := filepath.Join(root, "gone.md")

	cfg := &Config{MinWords: 1, MinChars: 1}
	record, keep, err := collectFile(cfg, SourceConfig{Name: "seed"}, missing, "gone.md", root)
	if err != nil {
		t.Fatalf("collectFile: unexpected error for a stat failure: %v", err)
	}
	if keep {
		t.Fatal("collectFile: keep = true, want false for a stat failure")
	}
	if record != (Record{}) {
		t.Fatalf("collectFile: record = %+v, want zero value", record)
	}
}

// TestCollectFile_ReadError_SkippedNotFatal covers collectFile's fallback
// bytelimit.ReadFileLimited error branch directly: a path that passes the
// Stat-based size pre-check but then fails to read must be skipped, not
// treated as fatal — the same reasoning as the stat-failure and
// oversized-file cases above. A directory Stats successfully (size 0,
// under the cap) but fails to Read as a file ("is a directory"),
// deterministically reaching this branch regardless of the running
// user's privileges (unlike a permission-bit test, which root ignores).
func TestCollectFile_ReadError_SkippedNotFatal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	notAFile := filepath.Join(root, "not-a-file.md")
	if err := os.Mkdir(notAFile, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := &Config{MinWords: 1, MinChars: 1}
	record, keep, err := collectFile(cfg, SourceConfig{Name: "seed"}, notAFile, "not-a-file.md", root)
	if err != nil {
		t.Fatalf("collectFile: unexpected error reading a directory as a file: %v", err)
	}
	if keep {
		t.Fatal("collectFile: keep = true, want false when the read fails")
	}
	if record != (Record{}) {
		t.Fatalf("collectFile: record = %+v, want zero value", record)
	}
}

// --- reportProgress ---

// TestReportProgress pins all three branches: nil cfg is a no-op,
// non-nil cfg with nil Progress is a no-op, and non-nil cfg with
// a Progress callback receives the message verbatim.
func TestReportProgress(t *testing.T) {
	t.Parallel()
	t.Run("nil cfg is no-op", func(t *testing.T) {
		reportProgress(nil, "ignored")
	})
	t.Run("nil Progress is no-op", func(t *testing.T) {
		reportProgress(&Config{}, "ignored")
	})
	t.Run("Progress callback receives message", func(t *testing.T) {
		var got string
		cfg := &Config{Progress: func(s string) { got = s }}
		reportProgress(cfg, "collecting alpha")
		if got != "collecting alpha" {
			t.Errorf("got %q, want %q", got, "collecting alpha")
		}
	})
}

// --- sourceRelativePath ---

// TestSourceRelativePath pins every reachable branch: empty and
// absolute configured roots pass through, relative roots prepend
// with slash-style joining, and the "./" / leading "/" trims fire
// on the recognised shapes. The unreachable `joined == ""`
// fallback is defensive — filepath.Join always cleans to "." or
// a non-empty path given non-empty inputs — and is not driven.
func TestSourceRelativePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		root    string
		rel     string
		resRoot string
		want    string
	}{
		{name: "empty configured root passes through",
			root: "", rel: "docs/file.md", want: "docs/file.md"},
		{name: "whitespace-only root passes through",
			root: "   ", rel: "file.md", want: "file.md"},
		{name: "absolute configured root passes through",
			root: "/abs/docs", rel: "file.md", want: "file.md"},
		{name: "relative root joins and slash-cleans",
			root: "docs", rel: "guide.md", want: "docs/guide.md"},
		{name: "dot-relative root trims ./",
			root: "./docs", rel: "guide.md", want: "docs/guide.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sourceRelativePath(c.root, c.rel, c.resRoot)
			if got != c.want {
				t.Errorf("sourceRelativePath(%q,%q,%q) = %q, want %q",
					c.root, c.rel, c.resRoot, got, c.want)
			}
		})
	}
}
