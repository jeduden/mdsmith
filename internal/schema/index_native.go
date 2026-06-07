//go:build !wasm

package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// This file holds the schema-index side-output read and write paths.
// They touch the OS filesystem (os.MkdirAll, os.CreateTemp, os.Rename,
// os.ReadFile, …), so they are built only on native (//go:build
// !wasm). The WASM build runs against an in-memory MemWorkspace with
// no OS disk, so it replaces WriteIndex and ValidateIndex with no-op
// stubs (index_wasm.go): the `<?index?>` sidecar feature is inert
// there. See docs/background/concepts/engine-api.md.

// WriteIndex writes the JSON index produced by BuildIndex next to
// the source file. Output paths are resolved relative to the source
// file's directory; absolute paths (including Windows drive-letter
// and leading-backslash forms), parent-traversal segments, and
// symlinks that escape the allowed root are rejected so a schema
// cannot trick fix into writing outside the project. Parent
// directories are created on demand so a nested `output:` path
// (e.g. `.mdsmith/index/runbook.json`) works on a clean checkout.
//
// The allowed root is f.RootDir when set (the project root), and
// the source file's directory otherwise. After mkdir we
// EvalSymlinks the parent directory and verify it still resolves
// inside that root, so a `sub` directory that turns out to be a
// symlink to `/etc` is caught before any bytes are written.
//
// The target file itself is also Lstat-checked: if an existing
// symlink sits at the index path (an in-root symlink that points
// outside the project — e.g. `.runbook-index.json` →
// `/etc/passwd`), os.WriteFile would follow it and clobber the
// link target. We reject the write instead. The write goes through
// a sibling temp file + os.Rename so the directory entry is
// replaced atomically and never as a symlink-follow operation.
//
// On error WriteIndex records the failure in the package-level
// indexWriteErr cache keyed by f.Path so the next Check surfaces
// the underlying I/O error instead of repeating the generic
// "missing / out of date" message — otherwise a misconfiguration
// (e.g. `output: "."` resolving to a directory) would trap users
// in a fix loop with no signal about what is actually wrong.
// A successful write clears the entry.
func WriteIndex(f *lint.File, sch *Schema) error {
	target, data, err := resolveIndexWrite(f, sch)
	if err != nil {
		recordIndexWriteError(f, err)
		return err
	}
	if data == nil {
		recordIndexWriteError(f, nil)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		err = fmt.Errorf("schema.index: create parent dir: %w", err)
		recordIndexWriteError(f, err)
		return err
	}
	if err := verifyIndexWithinRoot(f, target); err != nil {
		recordIndexWriteError(f, err)
		return err
	}
	if err := rejectSymlinkTarget(target); err != nil {
		recordIndexWriteError(f, err)
		return err
	}
	if err := atomicWriteIndex(target, data); err != nil {
		recordIndexWriteError(f, err)
		return err
	}
	recordIndexWriteError(f, nil)
	return nil
}

// rejectSymlinkTarget refuses to write when the index path already
// exists as a symlink. os.WriteFile follows symlinks and would
// overwrite whatever the link points at; an in-root link pointing
// outside the project (e.g. `.runbook-index.json` -> `/etc/passwd`)
// would defeat verifyIndexWithinRoot, which only checks the parent
// directory. A non-symlink existing file is fine — atomic
// replacement is the normal Fix path.
func rejectSymlinkTarget(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("schema.index: stat target %q: %w", target, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf(
			"schema.index.output target %q is a symlink; refusing to write "+
				"to avoid clobbering the symlink's destination",
			target)
	}
	return nil
}

// atomicWriteIndex writes data to a sibling temp file in target's
// directory and renames it into place. os.Rename replaces the
// directory entry without following any symlink that may have
// raced into the target after rejectSymlinkTarget ran, and yields
// a torn-write-free result on POSIX filesystems.
func atomicWriteIndex(target string, data []byte) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".mdsmith-index-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := writeAndRename(tmp, tmpPath, target, data); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func writeAndRename(tmp *os.File, tmpPath, target string, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}

// verifyIndexWithinRoot resolves the symlinks on target's parent
// directory and reports an error if the resolved path escapes the
// allowed root (f.RootDir when set, otherwise the source file's
// directory). The target file itself need not exist; only its
// parent must, and MkdirAll has just been called so it does. Hosts
// that cannot EvalSymlinks the parent (e.g. permission failures)
// fall back to a Clean-based comparison — best-effort rather than
// airtight, but still beats no check.
func verifyIndexWithinRoot(f *lint.File, target string) error {
	root := f.RootDir
	if root == "" {
		root = filepath.Dir(f.Path)
	}
	resolvedRoot := resolveDir(root)
	resolvedParent := resolveDir(filepath.Dir(target))
	rel, err := filepath.Rel(resolvedRoot, resolvedParent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf(
			"schema.index.output resolves to %q which is outside the "+
				"allowed root %q (symlink escape rejected)",
			resolvedParent, resolvedRoot)
	}
	return nil
}

// resolveDir absolutises and follows symlinks on dir, falling back
// to filepath.Clean(filepath.Abs(...)) when EvalSymlinks fails
// (e.g. a path component lacks read permission). filepath.Abs only
// fails when the host has no working directory, which is not a
// recoverable state, so we ignore its error and rely on Clean.
func resolveDir(dir string) string {
	abs, _ := filepath.Abs(dir)
	if abs == "" {
		abs = filepath.Clean(dir)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

// ValidateIndex compares the on-disk index file (if any) against
// the bytes BuildIndex would emit. When the file is missing or its
// content differs, a diagnostic asks the user to run `mdsmith fix`
// so the artefact stays in sync. Comparison normalises CRLF line
// endings to LF so a Windows checkout with `core.autocrlf=true`
// does not flag a semantically-identical file as stale. Read
// errors other than "file does not exist" surface as a distinct
// diagnostic. If the last Fix tried to write this index and
// failed, the cached I/O error is reported in place of the generic
// "missing / out of date" message so users can act on the real
// cause instead of running fix again. `mdsmith check` still
// respects the read-only contract: it never touches the file.
func ValidateIndex(f *lint.File, sch *Schema, mkDiag MakeDiag) []lint.Diagnostic {
	target, want, err := resolveIndexWrite(f, sch)
	if err != nil {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf("index: %v", err))}
	}
	if want == nil {
		// A schema that previously declared an index: but no longer
		// does should not leave stale write-error entries lying
		// around in a long-running process (notably the LSP server).
		recordIndexWriteError(f, nil)
		return nil
	}
	if writeErr := lastIndexWriteError(f); writeErr != nil {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q write failed on the last `mdsmith fix`: %v",
				sch.Index.Output, writeErr))}
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return []lint.Diagnostic{mkDiag(f.Path, 1,
				fmt.Sprintf(
					"index side-output %q is missing; run `mdsmith fix`",
					sch.Index.Output))}
		}
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q cannot be read: %v",
				sch.Index.Output, readErr))}
	}
	if !indexContentEqual(got, want) {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q is out of date; run `mdsmith fix`",
				sch.Index.Output))}
	}
	return nil
}
