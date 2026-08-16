---
id: 2608161754
title: >-
  Vendor go-runewidth as a patched fork and bump tinygo
  to 0.41.1
status: "🔲"
model: sonnet
summary: >-
  go-runewidth 0.0.27 builds a 2.1 MiB strictWidthLUT in
  package init; tinygo's interp pass tries to constant-fold
  it and times out, so the wasm build fails and the
  dependency is stuck at 0.0.24. Vendor the library into
  pkg/runewidth with its MIT LICENSE, drop the eager LUT,
  and gate the fork with an upstream A/B parity test wired
  into CI. Also bump the pinned tinygo to 0.41.1.
depends-on: []
---
# Vendor go-runewidth as a patched fork and bump tinygo to 0.41.1

## Goal

Own a patched copy of go-runewidth. The wasm build then
stops failing on an upstream change mdsmith cannot
otherwise influence.

## Background

Dependabot [PR #777][pr777] bumps go-runewidth 0.0.24 to
0.0.27. The `tinygo-wasm` CI job fails on it:

```text
runewidth.go:118:6: interp: running for more than 3m0s,
timing out (executed calls: 189515)
```

The traceback names the cause: `init#1` calls
`initStrictWidthLUT`. Upstream 0.0.27 added a precomputed
width lookup table, declared as
`strictWidthLUT [2][0x110000]byte` and filled in package
`init`. That is 2,228,224 entries, each derived by calling
`runeWidthNoLUT`. Tinygo's `interp` pass tries to evaluate
the whole initializer at compile time and gives up at its
3-minute budget.

Bumping tinygo alone does not fix it. Measured on CI:
0.39.0 reached 102,156 interpreted calls in 3 minutes and
0.41.1 reached 189,515 — roughly 1.85 times faster, against
a workload of millions of calls. Raising
`-interp-timeout` would charge every future CI run ten
minutes or more for a dependency bump that carries no
security fix.

Two further facts make vendoring the right shape:

- The LUT is a 2.1 MiB static array. The wasm artifact
  budget in [size_test.go][sizetest] is 8 MiB raw, so the
  table alone would consume about a quarter of it even if
  the compile-time problem were solved.
- mdsmith uses exactly one symbol from the library,
  `runewidth.StringWidth`, from three call sites:
  [tablefmt.go][tablefmt], [parity.go][parity], and
  [rule_test.go][tabletest].

A precedent already exists. [pkg/goldmark][goldmark] is a
vendored fork carrying its upstream LICENSE, with a
`goldmark_upstream` build tag that CI exercises through the
`goldmark-fork-test` job. This plan follows that shape.

It also avoids a known trap. The archived audit records
that [internal/mdtext/upstreampunct.go][punct] carries a
`mdtext_punkt_upstream` tag no workflow ever passes, and
calls it a "silent bit-rot risk". The parity tag added here
must run in CI from the first commit.

Vendoring also retires [PR #777][pr777] and every future
dependabot PR for this module, because the `go.mod`
requirement goes away.

## Tasks

1. Bump the pinned tinygo in [ci.yml][ci] from 0.39.0 to
   0.41.1. Set `TINYGO_SHA256` to
   `901fbccffc61adb111656d1f907dfc21b6c499ca841b115866a0d6de2d835fbe`,
   computed from the 179,764,108-byte release artifact.
   Comment why the pin is coupled to go-runewidth.
2. Copy go-runewidth 0.0.27 into `pkg/runewidth/`: the
   non-test `.go` files plus `LICENSE`. Record the upstream
   version and commit in a package doc comment. Do not copy
   `go.mod`, `go.sum`, or the benchmark scratch files.
3. Rewrite the package clause and any internal imports for
   the new path. Keep the exported API byte-identical so a
   future re-sync stays a mechanical diff.
4. Write the failing parity test first, behind a
   `runewidth_upstream` build tag: for every rune in
   `0..0x10FFFF`, assert the vendored `StringWidth` agrees
   with upstream 0.0.27. It fails until step 5 lands.
5. Remove the eager LUT from the fork: delete
   `strictWidthLUT`, delete `initStrictWidthLUT`, drop its
   call from `init`, and route the strict-width path
   through `runeWidthNoLUT`. Do not substitute a
   `sync.Once`: that keeps the 2.1 MiB array and only moves
   the cost to the first call.
6. Add a `runewidth-fork-test` job to [ci.yml][ci] running
   `go test -tags runewidth_upstream ./pkg/runewidth/...`,
   mirroring `goldmark-fork-test`. This is the step that
   keeps the fork honest.
7. Repoint the three call sites to the vendored package and
   drop `github.com/mattn/go-runewidth` from `go.mod` and
   `go.sum`.
8. Record the fork and its divergence in
   [markdown-library.md][mdlib] or a sibling development
   page, so the next person to re-sync knows the LUT
   removal is deliberate.
9. Close [PR #777][pr777] as retired by this work.

## Acceptance Criteria

- [ ] `tinygo build -target wasm ./cmd/mdsmith-wasm`
      succeeds with the vendored package, with no interp
      timeout.
- [ ] `TestTinyGoWASMArtifactSizeBudget` passes, and the
      artifact does not grow by the 2.1 MiB the LUT would
      have added.
- [ ] The parity test proves vendored and upstream
      `StringWidth` agree across the full rune range, and
      fails if the fork's behavior drifts.
- [ ] CI runs the parity tag in its own job, so the tag
      cannot rot the way `mdtext_punkt_upstream` did.
- [ ] `pkg/runewidth/LICENSE` is the upstream MIT license,
      unmodified, with copyright intact.
- [ ] `github.com/mattn/go-runewidth` no longer appears in
      `go.mod` or `go.sum`.
- [ ] Table formatting output is unchanged: the
      `tableformat` and `tablefmt` suites pass untouched.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[pr777]: https://github.com/jeduden/mdsmith/pull/777
[sizetest]: ../cmd/mdsmith-wasm/size_test.go
[tablefmt]: ../internal/rules/tablefmt/tablefmt.go
[parity]: ../internal/release/parity.go
[tabletest]: ../internal/rules/tableformat/rule_test.go
[goldmark]: ../pkg/goldmark/markdown.go
[punct]: ../internal/mdtext/upstreampunct.go
[ci]: ../.github/workflows/ci.yml
[mdlib]: ../docs/development/markdown-library.md
