//go:build !goldmark_upstream

package integration

// mds043AllocCeiling is MDS043's allocs/op ceiling on the default
// arena build of the vendored goldmark fork (plan 198).
//
// Plan 188 removed MDS043's second goldmark parse:
// collectReferenceDefinitions now reads labels from
// f.LinkReferences() (the canonical-parse reference set, already
// memoized) and locates each definition with a byte scanner instead
// of a fresh parse + full-source regex. The second parse was the
// whole reason this ceiling was build-tag split — the arena
// allocator made that parse roughly half as expensive as the
// non-arena build. With it gone, the per-rule bench doc (inline
// links only, no reference definitions) charges MDS043 only its AST
// walks: baseline ~10 allocs/op on BOTH build axes. The split files
// now pin the SAME value; the indirection is kept so a future
// axis-specific divergence has a home.
//
// Ceiling 16 = baseline 10 + headroom (well over max(20%, 4)), tight
// enough to catch a reintroduced parse or a lost LinkReferences memo.
const mds043AllocCeiling float64 = 16
