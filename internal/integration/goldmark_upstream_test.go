//go:build goldmark_upstream

package integration

// mds043AllocCeiling is MDS043's allocs/op ceiling on the non-arena
// (upstream) build of the vendored goldmark fork (plan 198). See the
// goldmark_arena_test.go variant for the rationale. This file is
// selected under `-tags goldmark_upstream`, where the arena is off and
// MDS043's second parse (collectReferenceDefinitions) allocates
// roughly twice as much: baseline ~653 allocs, ceiling 784 = base +
// 20%.
const mds043AllocCeiling float64 = 784
