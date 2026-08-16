---
id: 2608161754
title: >-
  Vendor go-runewidth as a patched fork so post-LUT
  versions build under tinygo
status: "🔲"
model: sonnet
summary: >-
  go-runewidth 0.0.25+ fills a 2.1 MiB strictWidthLUT in
  package init; tinygo's interp pass constant-folds it and
  times out, freezing mdsmith at 0.0.24. The LUT is a pure
  memoization of the runeWidthNoLUT function it keeps, so a
  vendored fork can delete it with an equivalence-by-
  construction regression test. Vendor 0.0.27 into
  pkg/runewidth with its MIT LICENSE, delete the LUT,
  promote the uax29 grapheme dependency to direct, and drop
  the module. Retires PR #777.
depends-on: []
---
# Vendor go-runewidth as a patched fork so post-LUT versions build under tinygo

## Goal

Own a patched copy of go-runewidth with the eager width
lookup table removed. mdsmith can then adopt current and
future upstream versions, instead of staying pinned at
0.0.24.

## Background

Dependabot [PR #777][pr777] bumps go-runewidth 0.0.24 to
0.0.27. The `tinygo-wasm` CI job fails on it:

```text
runewidth.go:118:6: interp: running for more than 3m0s,
timing out (executed calls: 189515)
```

Upstream 0.0.25 added a precomputed width lookup table. It
is declared `strictWidthLUT [2][0x110000]byte` and filled
in package `init` — 2,228,224 entries. Tinygo's `interp`
pass tries to evaluate the whole initializer at compile
time. It gives up at its 3-minute budget.

**Why not just stay on 0.0.24.** That is the zero-work
option, and 0.0.24 has no LUT (verified: no `strictWidthLUT`
in its source), so CI is green on it today. But every
go-runewidth release from 0.0.25 onward carries the LUT
permanently, so without a fork mdsmith is frozen at 0.0.24
for good — including for the grapheme-cluster and
spacing-mark width corrections 0.0.27 brings. Vendoring is
the forward path; pinning is the dead end.

**The fix is mechanical.** The LUT is not new behavior — it
is a pure cache of a function the fork keeps:

```go
func initStrictWidthLUT() {
    for i := range strictWidthLUT[0] {
        strictWidthLUT[0][i] = byte(runeWidthNoLUT(rune(i), false, true))
        strictWidthLUT[1][i] = byte(runeWidthNoLUT(rune(i), true, true))
    }
}
func (c *Condition) RuneWidth(r rune) int {
    // ... return int(strictWidthLUT[k][r])
    // ... fallback: return runeWidthNoLUT(r, c.EastAsianWidth, c.StrictEmojiNeutral)
}
```

Deleting the array and routing every read back through
`runeWidthNoLUT` is equivalent by construction. This is a
memoization removal, not a behavior fork. So it needs a
regression test, not the upstream-A/B build-tag apparatus
[pkg/goldmark][goldmark] uses. Goldmark keeps two real
runtime paths, arena on and off, that must stay in
agreement. There is no alternate path to keep here. The LUT
path simply ceases to exist.

**This is still a behavior change, but a different one.**
Moving from 0.0.24 to 0.0.27 changes some per-rune and
grapheme-cluster widths — that is what upstream's fixes do.
Table alignment for emoji/CJK content may shift. That is a
deliberate upgrade, not a regression, and it is separate
from the LUT removal, which is width-preserving.

**A second dependency comes along.**
`runewidth.StringWidth` routes multi-rune input through
`github.com/clipperhouse/uax29/v2/graphemes`. This is
verified in both 0.0.24 and 0.0.27. uax29 is `// indirect`
in [go.mod][gomod] today. Once go-runewidth is removed, it
becomes a direct dependency.

Its grapheme trie is generated lookup code, with no `init`
and no large static array. That makes it a low tinygo risk,
though the risk must still be confirmed by a real tinygo
build. uax29 keeps its own dependabot line, which this work
does not retire.

mdsmith uses exactly one symbol, `runewidth.StringWidth`.
It has three call sites: [tablefmt.go][tablefmt],
[parity.go][parity], and [rule_test.go][tabletest]. So the
fork surface and the repoint are both small.

The archived audit flags a trap to avoid.
[internal/mdtext/upstreampunct.go][punct]'s
`mdtext_punkt_upstream` tag is one no workflow runs — a
"silent bit-rot risk." This plan sidesteps it by design.
The LUT regression test is an ordinary untagged test in the
default suite. It always runs, so it cannot rot.

## Tasks

1. Spike first: vendor go-runewidth 0.0.27 into
   `pkg/runewidth/` (non-test `.go` files plus `LICENSE`),
   rewrite the package path, promote uax29 to a direct
   `require`, and confirm `tinygo build -target wasm
   ./cmd/mdsmith-wasm` still times out — establishing the
   reproduction inside the tree before changing anything.
   If tinygo instead fails on uax29, stop and re-plan.
2. Record the upstream version and commit in a package doc
   comment. Do not copy `go.mod`, `go.sum`, or the
   benchmark scratch files. Keep the exported API
   byte-identical so a future re-sync stays a mechanical
   diff.
3. Write the regression test first (ordinary test, no build
   tag): for every rune in `0..0x10FFFF` and both
   `eastAsian` settings, assert `RuneWidth` equals
   `runeWidthNoLUT` with the same arguments; add a handful
   of `StringWidth` cases exercising the uax29 grapheme
   path (a ZWJ emoji sequence, a regional-indicator flag, a
   base+combining sequence). It passes on the as-copied
   fork and must still pass after step 4 — it guards the
   removal, it does not drive it.
4. Delete the eager LUT: remove `strictWidthLUT`, remove
   `initStrictWidthLUT`, drop its call from `init`, and
   replace every `strictWidthLUT[k][r]` read with the
   equivalent `runeWidthNoLUT(r, eastAsian, strictEmojiNeutral)`
   call, preserving the exact arguments each read encoded.
   Do not substitute a `sync.Once`: that keeps the 2.1 MiB
   array and only moves the fill to first call.
5. Repoint the three call sites to `pkg/runewidth` and drop
   `github.com/mattn/go-runewidth` from [go.mod][gomod] and
   `go.sum`. Confirm uax29 is now a direct requirement.
6. Regenerate the table-formatting golden fixtures if the
   0.0.24 -> 0.0.27 width changes move any output, and
   review the diff so the shift is understood and intended,
   not silently absorbed.
7. Document the fork and its single divergence (the LUT
   removal) in [markdown-library.md][mdlib] or a sibling
   development page, so a re-sync knows the deletion is
   deliberate.
8. Close [PR #777][pr777] as retired by this work.

## Optional follow-up (not required by this plan)

Bump the pinned tinygo in [ci.yml][ci] from 0.39.0 to
0.41.1. It is independently worthwhile — newer toolchain,
LLVM 20, and roughly 1.85x faster interp (measured on CI:
0.39.0 reached 102,156 calls in the 3-minute budget, 0.41.1
reached 189,515). It is **not** needed for this fix: once
the LUT is gone, 0.39.0 builds the engine fine. Keep it a
separate change so a toolchain regression cannot be
mistaken for a vendoring regression. Its `TINYGO_SHA256` is
supplied at merge time per project practice.

## Acceptance Criteria

- [ ] `tinygo build -target wasm ./cmd/mdsmith-wasm`
      succeeds with the vendored package, no interp timeout.
- [ ] `TestTinyGoWASMArtifactSizeBudget` passes and the
      artifact does not carry the 2.1 MiB the LUT would add.
- [ ] The regression test proves `RuneWidth` equals
      `runeWidthNoLUT` across the full rune range and both
      `eastAsian` settings, and covers a ZWJ emoji, a flag,
      and a combining sequence through `StringWidth`.
- [ ] The regression test is untagged and runs in the
      default `go test ./...`, so it cannot rot the way
      `mdtext_punkt_upstream` did.
- [ ] `pkg/runewidth/LICENSE` is the upstream MIT license,
      unmodified, copyright intact.
- [ ] `github.com/mattn/go-runewidth` no longer appears in
      `go.mod` or `go.sum`; `uax29/v2` is a direct require.
- [ ] Any table-formatting output change from the
      0.0.24 -> 0.0.27 upgrade is captured in regenerated
      fixtures and reviewed, not silently applied.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[pr777]: https://github.com/jeduden/mdsmith/pull/777
[gomod]: ../go.mod
[tablefmt]: ../internal/rules/tablefmt/tablefmt.go
[parity]: ../internal/release/parity.go
[tabletest]: ../internal/rules/tableformat/rule_test.go
[goldmark]: ../pkg/goldmark/markdown.go
[punct]: ../internal/mdtext/upstreampunct.go
[ci]: ../.github/workflows/ci.yml
[mdlib]: ../docs/development/markdown-library.md
