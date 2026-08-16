---
id: 2608161754
title: >-
  Vendor go-runewidth as a patched fork so post-LUT
  versions build under tinygo
status: "🔳"
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

## Performance impact

Removing the LUT is a net win for mdsmith's workload, not a
regression. The numbers below were measured on this
machine. They compare 0.0.27 as copied against the same
code with the LUT deleted.

| Path                           | With LUT | No LUT |
| ------------------------------ | -------- | ------ |
| Package `init` (every startup) | 12.7 ms  | 0 ms   |
| Static array                   | 2.1 MiB  | 0      |
| `StringWidth`, ASCII cell      | 7.8 ns   | 6.5 ns |
| `StringWidth`, CJK cell        | 124 ns   | 222 ns |
| `RuneWidth`, one CJK rune      | 1.4 ns   | 9.1 ns |

The LUT's cost is a fixed 12.7 ms fill at every process
start, plus 2.1 MiB resident. The CLI starts fresh on every
invocation, so it pays that on `mdsmith version` and on
every file with no tables at all. The benefit is roughly
100 ns saved per CJK table cell. Around 127,000 such cells
per run are needed just to offset one startup fill.

**Optimize for English content — the deciding factor.**
`StringWidth` has an all-ASCII fast path in both 0.0.24 and
0.0.27: a pure-ASCII string counts printable bytes and never
calls `RuneWidth`. So English text never reads the LUT at
all. The LUT gives English content zero per-call benefit,
while every run still pays its 12.7 ms startup fill. Removed,
English `StringWidth` is measurably faster and consistently
so (about 6.5 ns against 8.0 ns per cell across five runs).

mdsmith's own corpus and most Markdown tables are English or
otherwise ASCII, so this is the case to optimize. The only
content the LUT helps is CJK and emoji, and only in the
per-rune width step that uax29 grapheme clustering already
dominates.

**Emoji specifically, since we write them.** The status
emoji in plan front matter and the PLAN.md status column —
`✅ 🔲 🔳 ⛔ 🤖` — measure width 2 in every variant: 0.0.24,
0.0.27 with the LUT, and 0.0.27 without it. Removing the LUT
changes no emoji width, which the parity test below proves
across the whole rune range. So emoji tables do not
re-align from this change. Per emoji cell the no-LUT path
costs about 7 ns more, and a 250-row status column adds
roughly 2 microseconds per format. That is accepted.

One real width change does come from the version bump, not
the LUT: regional-indicator flags such as `🇺🇸` go from
width 1 in 0.0.24 to width 2 in 0.0.27, an upstream grapheme
fix. Any table with flag emoji re-aligns once, by design.
The regression test pins the status emoji and a flag so this
behavior is locked and a future re-sync cannot regress it.

Against the current pinned 0.0.24, the width path is
unchanged: 0.0.24 has no LUT either. A parity test confirms
the LUT-free fork returns identical widths across all
1,114,112 runes and both East-Asian settings.

## Tasks

The work is three phases. Phase 0 is a spike whose result
gates the other two — do not start Phase 1 until it passes.

### Phase 0 — spike (go / no-go)

One question decides whether the rest of the plan holds:
does uax29 build under tinygo? Everything else is already
known — the LUT times out (seen on [PR #777][pr777]'s CI),
the removal is equivalence-by-construction, and the widths
are measured. So spend the smallest effort that answers only
the open question, before writing any of the real change.

1. Vendor go-runewidth 0.0.27 into `pkg/runewidth/` (non-test
   `.go` files plus `LICENSE`), rewrite the package path, and
   promote uax29 to a direct `require`.
2. Run `tinygo build -target wasm ./cmd/mdsmith-wasm` and read
   which package the interp pass dies in.

The failure location is the gate:

- Timeout in `runewidth` (expected) — the LUT reproduces
  inside the tree and uax29 compiles. Proceed to Phase 1 on
  this same branch.
- Failure in `uax29` — the plan's shape is wrong. Stop.
  uax29 then needs its own treatment (vendor and patch it
  too, or keep grapheme clustering off the wasm path), which
  is a different plan. Do not continue.

### Phase 1 — remove the LUT

3. Record the upstream version and commit in a package doc
   comment. Do not copy `go.mod`, `go.sum`, or the benchmark
   scratch files. Keep the exported API byte-identical so a
   future re-sync stays a mechanical diff.
4. Write the regression test first (ordinary test, no build
   tag): for every rune in `0..0x10FFFF` and both `eastAsian`
   settings, assert `RuneWidth` equals `runeWidthNoLUT` with
   the same arguments; add `StringWidth` cases for the emoji
   mdsmith actually writes — the status set `✅ 🔲 🔳 ⛔ 🤖`
   (each expected width 2) — plus the uax29 grapheme path (a
   ZWJ family sequence, a regional-indicator flag, a
   base+combining sequence). It passes on the as-copied fork
   and must still pass after step 5 — it guards the removal,
   it does not drive it.
5. Delete the eager LUT: remove `strictWidthLUT`, remove
   `initStrictWidthLUT`, drop its call from `init`, and
   replace every `strictWidthLUT[k][r]` read with the
   equivalent `runeWidthNoLUT(r, eastAsian, strictEmojiNeutral)`
   call, preserving the exact arguments each read encoded.
   Do not substitute a `sync.Once`: that keeps the 2.1 MiB
   array and only moves the fill to first call.
6. Confirm the tinygo wasm build now succeeds and
   `TestTinyGoWASMArtifactSizeBudget` passes — the Phase 0
   timeout is gone and the 2.1 MiB is no longer in the
   artifact.

### Phase 2 — integrate and retire the module

7. Repoint the three call sites to `pkg/runewidth` and drop
   `github.com/mattn/go-runewidth` from [go.mod][gomod] and
   `go.sum`. Confirm uax29 is now a direct requirement.
8. Regenerate the table-formatting golden fixtures if the
   0.0.24 -> 0.0.27 width changes move any output, and review
   the diff so the shift (flag emoji, say) is understood and
   intended, not silently absorbed.
9. Document the fork and its single divergence (the LUT
   removal) in [markdown-library.md][mdlib] or a sibling
   development page, so a re-sync knows the deletion is
   deliberate.
10. Close [PR #777][pr777] as retired by this work.

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

The two tinygo criteria are verified by CI's `tinygo-wasm`
job (tinygo is not installable in the dev environment); the
rest are verified locally.

- [ ] `tinygo build -target wasm ./cmd/mdsmith-wasm`
      succeeds with the vendored package, no interp timeout.
      (CI `tinygo-wasm`.)
- [ ] `TestTinyGoWASMArtifactSizeBudget` passes and the
      artifact does not carry the 2.1 MiB the LUT would add.
      (CI `tinygo-wasm`.)
- [x] The regression test proves `RuneWidth` equals
      `runeWidthNoLUT` across the full rune range and both
      `eastAsian` settings, and covers a ZWJ emoji, a flag,
      and a combining sequence through `StringWidth`.
- [x] The regression test is untagged and runs in the
      default `go test ./...`, so it cannot rot the way
      `mdtext_punkt_upstream` did.
- [x] `pkg/runewidth/LICENSE` is the upstream MIT license,
      unmodified, copyright intact.
- [x] `github.com/mattn/go-runewidth` no longer appears in
      `go.mod` or `go.sum`; `uax29/v2` is a direct require.
- [x] Any table-formatting output change from the
      0.0.24 -> 0.0.27 upgrade is captured in regenerated
      fixtures and reviewed, not silently applied. (Reviewed:
      no golden fixture moved — no fixture holds a
      width-changed rune — so none needed regenerating.)
- [x] `go test ./...` is green.
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [x] `mdsmith check .` is green.

[pr777]: https://github.com/jeduden/mdsmith/pull/777
[gomod]: ../go.mod
[tablefmt]: ../internal/rules/tablefmt/tablefmt.go
[parity]: ../internal/release/parity.go
[tabletest]: ../internal/rules/tableformat/rule_test.go
[goldmark]: ../pkg/goldmark/markdown.go
[punct]: ../internal/mdtext/upstreampunct.go
[ci]: ../.github/workflows/ci.yml
[mdlib]: ../docs/development/markdown-library.md
