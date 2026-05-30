//go:build !goldmark_upstream

package integration

// mds043AllocCeiling is MDS043's allocs/op ceiling on the default
// arena build of the vendored goldmark fork (plan 198). MDS043 is the
// only opt-in rule that parses the source a second time
// (collectReferenceDefinitions via LinkReferences), and the arena
// allocator is exactly what makes that second parse cheap: baseline
// ~320 allocs, ceiling 384 = baseline + max(20%, 4).
//
// The goldmark_upstream_test.go variant pins a higher ceiling for
// `-tags goldmark_upstream`, where the arena is off and the same
// second parse allocates roughly twice as much. Splitting the ceiling
// across the two build-tagged files keeps the tight arena-path gate
// intact while letting the non-arena A/B axis stay green, instead of
// blanket-raising one ceiling that would slacken the common path.
const mds043AllocCeiling float64 = 384
