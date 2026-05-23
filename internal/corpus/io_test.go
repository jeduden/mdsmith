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

// --- WriteManifest / WriteJSON / ensureParentDir error branches ---

// TestWriteManifest_ParentDirFailure pins the error path where
// ensureParentDir fails because the parent path is occupied by a
// regular file. The happy path is exercised by
// TestWriteManifest_WritesJSONL; the error wrap was not.
func TestWriteManifest_ParentDirFailure(t *testing.T) {
	t.Parallel()
	// A regular file at the would-be parent dir makes MkdirAll fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	bad := filepath.Join(blocker, "manifest.jsonl")
	err := WriteManifest(bad, nil)
	if err == nil {
		t.Fatal("expected error when parent path is a file")
	}
	if !strings.Contains(err.Error(), "create directory") {
		t.Errorf("error %q must mention create directory", err)
	}
}

// TestWriteJSON_ParentDirFailure mirrors the WriteManifest case
// for the WriteJSON wrapper, which shares the ensureParentDir
// pre-call.
func TestWriteJSON_ParentDirFailure(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	bad := filepath.Join(blocker, "report.json")
	err := WriteJSON(bad, struct{ X int }{1})
	if err == nil {
		t.Fatal("expected error when parent path is a file")
	}
	if !strings.Contains(err.Error(), "create directory") {
		t.Errorf("error %q must mention create directory", err)
	}
}

// TestEnsureParentDir pins both branches directly. Happy path:
// MkdirAll under a tempdir succeeds (nested path included).
// Error path: MkdirAll fails when an ancestor is a regular file.
func TestEnsureParentDir(t *testing.T) {
	t.Parallel()
	t.Run("happy path", func(t *testing.T) {
		nested := filepath.Join(t.TempDir(), "a", "b", "c", "f.json")
		if err := ensureParentDir(nested); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// The directory must now exist and be a directory.
		dir := filepath.Dir(nested)
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat parent: %v", err)
		}
		if !fi.IsDir() {
			t.Errorf("parent %s is not a directory", dir)
		}
	})
	t.Run("error path: ancestor is a regular file", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		err := ensureParentDir(filepath.Join(blocker, "child", "x.txt"))
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "create directory") {
			t.Errorf("error %q must mention create directory", err)
		}
	})
}
