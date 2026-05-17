package crossfilereferenceintegrity

import (
	"os"
	"path/filepath"
	"sync"
)

// MDS027 stats and symlink-resolves the same target and root paths
// repeatedly: once per link, per linting file, so a workspace check
// re-runs identical os.Stat / filepath.EvalSymlinks syscalls hundreds
// of times (Syscall6 was ~5.7% flat of check CPU, plan 175 profiling).
//
// The caches below are keyed by the resolved path and shared at package
// scope, NOT on the Rule struct: each parallel worker holds its own
// CloneInstance, and ConfigureRule may re-clone per file, so a
// per-instance map would neither share across workers nor be safe.
// sync.Map is the sanctioned concurrency-safe pattern here.
//
// Staleness caveat (mirrors the gitignore matcher cache): the result is
// stable for the lifetime of a `mdsmith check` process because the
// filesystem does not change under it. A long-lived LSP server that
// outlives on-disk edits would observe stale existence / symlink
// results; if MDS027 is ever wired into that path, the cache must gain
// an invalidation hook just as the gitignore cache would.
var (
	statExistsCache  sync.Map // map[string]bool
	evalSymlinkCache sync.Map // map[string]evalResult
)

type evalResult struct {
	real string
	ok   bool
}

// cachedStatExists reports whether path exists (os.Stat succeeds),
// memoizing the boolean. Only the existence outcome is consumed by
// callers, so caching it is exactly equivalent to the original
// `os.Stat(path); err == nil` check.
func cachedStatExists(path string) bool {
	if v, ok := statExistsCache.Load(path); ok {
		return v.(bool)
	}
	_, err := os.Stat(path)
	exists := err == nil
	statExistsCache.Store(path, exists)
	return exists
}

// cachedEvalSymlinks memoizes filepath.EvalSymlinks. It returns the
// resolved path and whether resolution succeeded; callers reproduce the
// original fallback (clean the input) when ok is false.
func cachedEvalSymlinks(path string) (string, bool) {
	if v, ok := evalSymlinkCache.Load(path); ok {
		r := v.(evalResult)
		return r.real, r.ok
	}
	real, err := filepath.EvalSymlinks(path)
	r := evalResult{real: real, ok: err == nil}
	evalSymlinkCache.Store(path, r)
	return r.real, r.ok
}
