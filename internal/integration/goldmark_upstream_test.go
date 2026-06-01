//go:build goldmark_upstream

package integration

// mds043AllocCeiling is MDS043's allocs/op ceiling on the non-arena
// (upstream) build of the vendored goldmark fork (plan 198). See the
// goldmark_arena_test.go variant for the full rationale. This file is
// selected under `-tags goldmark_upstream`.
//
// Plan 188 removed MDS043's second goldmark parse, which was the only
// reason this axis allocated more than the arena axis. With the
// second parse gone, MDS043's parse-subtracted Check on the per-rule
// bench doc (no reference definitions) is identical on both axes:
// baseline ~10 allocs/op. The two build-tagged files now pin the same
// ceiling; the split is retained only so a future divergence has a
// home.
const mds043AllocCeiling float64 = 16
