package build

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// snapshotCap bounds the number of directory entries a single snapshot
// may cover. Above it the snapshot is refused so a recipe pointed at a
// huge tree cannot make the post-condition check quadratically slow.
const snapshotCap = 2000

// fileState is the recorded metadata for one file in a snapshot. mtime is
// stored as a Unix-nanosecond value. hash is the sha256 of a regular
// file's content, captured eagerly: the before-snapshot must record the
// original bytes because they are gone once the recipe overwrites them,
// so the content-preserving-rewrite check (same size and mtime, different
// bytes) cannot be deferred. hash is zero for non-regular entries. link
// is the symlink target (empty for non-symlinks), captured via Readlink
// so a symlink is never followed.
type fileState struct {
	size  int64
	mtime int64
	mode  os.FileMode
	hash  [32]byte
	link  string
}

// snapshotDirs records the metadata of every entry in the (non-recursive)
// listing of each directory in dirs. Directories are de-duplicated.
// Symlinks are recorded via Lstat metadata plus Readlink and never
// followed. A total entry count above cap is a build error naming the
// offending directory.
func snapshotDirs(dirs []string, maxEntries int) (map[string]fileState, error) {
	snap := make(map[string]fileState)
	seen := make(map[string]struct{}, len(dirs))
	total := 0
	for _, dir := range dirs {
		if _, dup := seen[dir]; dup {
			continue
		}
		seen[dir] = struct{}{}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				// A not-yet-created output parent contributes nothing.
				continue
			}
			return nil, fmt.Errorf("scanning %s: %w", dir, err)
		}
		total += len(entries)
		if total > maxEntries {
			return nil, fmt.Errorf(
				"build snapshot scope exceeds 2 000 entries at %s; point outputs at a narrower directory",
				dir,
			)
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			st, err := statFile(path)
			if err != nil {
				return nil, err
			}
			snap[path] = st
		}
	}
	return snap, nil
}

// statFile records one path's metadata. A regular file is hashed eagerly
// (see fileState.hash); a symlink's target is read but never followed;
// other types record metadata only.
func statFile(path string) (fileState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return fileState{}, fmt.Errorf("inspecting %s: %w", path, err)
	}
	st := fileState{
		size:  info.Size(),
		mtime: info.ModTime().UnixNano(),
		mode:  info.Mode(),
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, lerr := os.Readlink(path)
		if lerr != nil {
			return fileState{}, fmt.Errorf("reading symlink %s: %w", path, lerr)
		}
		st.link = target
	case info.Mode().IsRegular():
		h, herr := hashFileSum(path)
		if herr != nil {
			return fileState{}, herr
		}
		st.hash = h
	}
	return st, nil
}

// hashFileSum returns the sha256 of a regular file's contents, streamed so a
// large artifact never has to fit in memory.
func hashFileSum(path string) ([32]byte, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from a directory listing we control
	if err != nil {
		return [32]byte{}, fmt.Errorf("hashing %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, fmt.Errorf("hashing %s: %w", path, err)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

// undeclaredWrite names a file outside the declared outputs whose state
// changed across the recipe.
type undeclaredWrite struct {
	path string
	kind string // "added", "removed", or "modified"
}

// diffSnapshots reports every file whose state differs between before and
// after, excluding paths in declared. A declared output legitimately
// changes; anything else is an undeclared write. Results are sorted by
// path for a deterministic diagnostic.
func diffSnapshots(before, after map[string]fileState, declared map[string]struct{}) []undeclaredWrite {
	var violations []undeclaredWrite
	for path, post := range after {
		if _, ok := declared[path]; ok {
			continue
		}
		pre, existed := before[path]
		if !existed {
			violations = append(violations, undeclaredWrite{path: path, kind: "added"})
			continue
		}
		if !sameState(pre, post) {
			violations = append(violations, undeclaredWrite{path: path, kind: "modified"})
		}
	}
	for path := range before {
		if _, ok := declared[path]; ok {
			continue
		}
		if _, stillThere := after[path]; !stillThere {
			violations = append(violations, undeclaredWrite{path: path, kind: "removed"})
		}
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].path < violations[j].path })
	return violations
}

// sameState reports whether two file states are equivalent: size, mtime,
// mode, content hash, and symlink target all match. The hash comparison
// catches a content-preserving rewrite (same size and mtime, different
// bytes); the cheap metadata fields short-circuit the common case.
func sameState(a, b fileState) bool {
	return a.size == b.size && a.mtime == b.mtime &&
		a.mode == b.mode && a.hash == b.hash && a.link == b.link
}
