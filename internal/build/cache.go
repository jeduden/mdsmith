package build

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CacheVersion is the schema version stored in the cache file and folded
// into every ActionID. Bumping it forces a single rebuild of every
// target after an upgrade.
const CacheVersion = 1

// cacheRelPath is the project-root-relative location of the build cache.
const cacheRelPath = ".mdsmith/build-cache.json"

// OutputHash pairs a declared output path with the sha256 of its content
// at build time. The path is project-root-relative and slash-normalized.
type OutputHash struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// CacheEntry is one target's record in the build cache. A target is
// identified by its sorted set of output paths; ActionID decides
// freshness.
type CacheEntry struct {
	Outputs  []OutputHash `json:"outputs"`
	Inputs   []string     `json:"inputs"`
	ActionID string       `json:"action-id"`
	Recipe   string       `json:"recipe"`
	BuiltAt  string       `json:"built-at"`
}

// Cache is the in-memory form of .mdsmith/build-cache.json.
type Cache struct {
	Version int          `json:"version"`
	Entries []CacheEntry `json:"entries"`
}

// NewCache returns an empty cache stamped with the current schema
// version.
func NewCache() *Cache {
	return &Cache{Version: CacheVersion}
}

// LoadCache reads .mdsmith/build-cache.json under root. A missing file
// yields an empty cache; a present but malformed file is an error so a
// poisoned cache surfaces loudly rather than silently rebuilding.
func LoadCache(root string) (*Cache, error) {
	path := filepath.Join(root, filepath.FromSlash(cacheRelPath))
	data, err := os.ReadFile(path) //nolint:gosec // path is the fixed in-root cache file
	if err != nil {
		if os.IsNotExist(err) {
			return NewCache(), nil
		}
		return nil, fmt.Errorf("reading build cache: %w", err)
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing build cache: %w", err)
	}
	if c.Version == 0 {
		c.Version = CacheVersion
	}
	return &c, nil
}

// outputSetKey joins a sorted set of output paths into a single
// length-framed key so two different sets cannot collide.
func outputSetKey(paths []string) string {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	var b strings.Builder
	for _, p := range sorted {
		fmt.Fprintf(&b, "%d:%s|", len(p), p)
	}
	return b.String()
}

// Lookup returns the entry whose output-path set equals the given set
// (order-independent), and whether one was found.
func (c *Cache) Lookup(outputPaths []string) (CacheEntry, bool) {
	want := outputSetKey(outputPaths)
	for _, e := range c.Entries {
		if outputSetKey(e.outputPaths()) == want {
			return e, true
		}
	}
	return CacheEntry{}, false
}

// outputPaths returns just the path field of each output hash.
func (e CacheEntry) outputPaths() []string {
	paths := make([]string, len(e.Outputs))
	for i, o := range e.Outputs {
		paths[i] = o.Path
	}
	return paths
}

// Put inserts or replaces the entry identified by its output-path set.
func (c *Cache) Put(entry CacheEntry) {
	key := outputSetKey(entry.outputPaths())
	for i, e := range c.Entries {
		if outputSetKey(e.outputPaths()) == key {
			c.Entries[i] = entry
			return
		}
	}
	c.Entries = append(c.Entries, entry)
}

// Save writes the cache to .mdsmith/build-cache.json under root via a
// temp file and atomic rename, so a mid-build crash leaves the previous
// cache readable. Entries are sorted by output-set key for stable diffs.
func (c *Cache) Save(root string) error {
	if c.Version == 0 {
		c.Version = CacheVersion
	}
	sort.SliceStable(c.Entries, func(i, j int) bool {
		return outputSetKey(c.Entries[i].outputPaths()) <
			outputSetKey(c.Entries[j].outputPaths())
	})

	dir := filepath.Join(root, ".mdsmith")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating .mdsmith dir: %w", err)
	}

	data, _ := json.MarshalIndent(c, "", "  ")
	data = append(data, '\n')

	final := filepath.Join(dir, "build-cache.json")
	if err := atomicWriteFile(final, 0o644, data); err != nil {
		return fmt.Errorf("committing build cache: %w", err)
	}
	return nil
}

// writeTempFileVar is the writeTempFile implementation used by
// atomicWriteFile; tests may replace it to inject write failures without
// file-system tricks.
var writeTempFileVar = writeTempFile

// chmodFileFn indirects the temp-file chmod in atomicWriteFile so a test
// can inject a chmod failure without filesystem tricks.
var chmodFileFn = func(f *os.File, mode os.FileMode) error {
	return f.Chmod(mode)
}

// writeTempFile writes data to wc and closes it. Extracted so tests
// can inject a failing io.WriteCloser without needing file-system tricks.
func writeTempFile(wc io.WriteCloser, data []byte) error {
	if _, err := wc.Write(data); err != nil {
		_ = wc.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	return nil
}

// atomicWriteFile writes data to final atomically: a temp file in the same
// directory (so the rename is same-volume), chmod'd to mode, then renamed
// over final. A crash mid-write leaves final untouched. Shared by the
// build cache and the trust marker so both get the same durability and
// the same test-injectable inner write.
func atomicWriteFile(final string, mode os.FileMode, data []byte) error {
	dir := filepath.Dir(final)
	tmp, err := os.CreateTemp(dir, filepath.Base(final)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // best-effort cleanup; harmless once rename succeeds
	if err := chmodFileFn(tmp, mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting temp file mode: %w", err)
	}
	if err := writeTempFileVar(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("committing %s: %w", filepath.Base(final), err)
	}
	return nil
}
