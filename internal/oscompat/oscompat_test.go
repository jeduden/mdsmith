//go:build !tinygo

package oscompat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/oscompat"
)

func TestChmod_SetsPermissions(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "chmod-test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := oscompat.Chmod(f.Name(), 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	fi, err := os.Stat(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 0600", got)
	}
}

func TestChmod_NonexistentFile_ReturnsError(t *testing.T) {
	err := oscompat.Chmod(filepath.Join(t.TempDir(), "no-such-file"), 0o600)
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestEvalSymlinks_ResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	got, err := oscompat.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if got != target {
		t.Errorf("EvalSymlinks(%q) = %q, want %q", link, got, target)
	}
}

func TestEvalSymlinks_NonexistentPath_ReturnsError(t *testing.T) {
	_, err := oscompat.EvalSymlinks(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Error("expected error for non-existent path, got nil")
	}
}

func TestSameFile_SameFile_ReturnsTrue(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "samefile")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	fi1, err := os.Stat(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	fi2, err := os.Stat(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !oscompat.SameFile(fi1, fi2) {
		t.Error("SameFile on same path returned false")
	}
}

func TestSameFile_DifferentFiles_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fi1, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	fi2, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	if oscompat.SameFile(fi1, fi2) {
		t.Error("SameFile on different files returned true")
	}
}
