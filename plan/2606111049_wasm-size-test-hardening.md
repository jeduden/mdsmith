---
id: 2606111049
title: Harden WASM size test to match production build
status: "🔳"
summary: >-
  size_test.go builds without -no-debug, so it
  tests a larger artifact than ships. Add
  -no-debug and a gzip-size check to match the
  production build.
model: ""
depends-on: [247]
---
# Harden WASM size test to match production build

## Goal

Make `TestTinyGoWASMArtifactSizeBudget` test the
artifact that actually ships. A budget breach in
CI should mean a real regression in deployed wasm
size.

## Context

The production build uses `-no-debug`, which
strips DWARF and can halve the output. The test
in
[`cmd/mdsmith-wasm/size_test.go`](../cmd/mdsmith-wasm/size_test.go)
invokes:

```sh
tinygo build -target wasm -o out .
```

That omits `-no-debug`, so the test artifact
includes debug info. Two problems follow.

First, a debug-only size breach triggers a
spurious CI failure. The stripped binary may be
well inside budget, but CI still fails.

Second, the test only checks raw bytes (≤ 8 MiB).
Browsers and CDNs deliver the `.wasm` file
compressed. A module can be under 8 MiB raw and
still exceed a reasonable transfer budget on
mobile.

## Design

Two changes to `size_test.go`.

### Match production flags

Add `-no-debug` to the `tinygo build` invocation:

```go
cmd := exec.Command("tinygo", "build",
    "-target", "wasm",
    "-no-debug",
    "-o", outPath,
    ".")
```

Extract the flags into a named `var` or `const`
so they cannot drift from the production script.

### Add a gzip size check

After the raw check, compress the binary with
`compress/gzip` (best speed) and assert the
gzip size is within a second budget.

Capture the baseline gzip size of the current
production binary first. Set `maxGzipBytes`
with 10% headroom above that baseline.

```go
const (
    maxRawBytes  = 8 * 1024 * 1024 // 8 MiB
    maxGzipBytes = 4 * 1024 * 1024 // 4 MiB — set after baseline
)
```

## Tasks

1. [x] Add `-no-debug` to the `tinygo build` call in
   [`cmd/mdsmith-wasm/size_test.go`](../cmd/mdsmith-wasm/size_test.go).
2. [x] Add a gzip-size assertion after the raw-size
   check. Use `compress/gzip` at best-speed.
3. [x] Measure the current production gzip size and
   set `maxGzipBytes` with 10% headroom.
4. [x] Add a comment explaining both budgets: raw
   guards tinygo heap and segment fit; gzip
   guards mobile transfer cost.

## Acceptance Criteria

- [x] The `tinygo build` call includes
      `-no-debug`
- [x] The test asserts both a raw-byte limit and
      a gzip-byte limit
- [x] `maxGzipBytes` is above the current
      production gzip size with ≥10% headroom
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
