---
id: 2606280208
title: External URL link checking rule (MDS072)
status: "✅"
summary: >-
  Add MDS072 `external-link-check` — an opt-in rule that validates external
  URLs by making HTTP HEAD (fallback GET) requests, caching results for the
  run, and reporting non-2xx responses as diagnostics.
model: opus
depends-on: [172]
---
# External URL link checking rule (MDS072)

## Goal

Add `MDS072 external-link-check`, a default-off rule that closes the gap with
gomarklint's `external-link` rule. MDS072 HEAD-checks every `http://` and
`https://` URL in the workspace, caches results per URL for the run, and
emits a diagnostic for each URL returning an error, a 4xx, or a 5xx.
Resolves [issue #47](https://github.com/jeduden/mdsmith/issues/47).

## Background

MDS027 (`cross-file-reference-integrity`) already validates local file and
heading links. External URLs are out of scope for MDS027 because they require
network I/O and are unsuitable for the hot default-on path. MDS072 is
off-by-default and opt-in per the same pattern as MDS068 (`link-style`).

`linkstyle.LinksConfig` already carries `external-skip`, a list of skip
patterns. Tests live in `rule_test.go` at line 268. MDS072 reads that key.
It adds two more: `external-timeout` and `external-rate-limit`.

## Design

### Config (via shared `links:` block)

```yaml
rules:
  external-link-check:
    enabled: true          # off by default
    links:
      external-skip:       # regex patterns; matching URLs are not checked
        - "^https?://localhost"
        - "^http://10\\."
      external-timeout: 10s  # per-request timeout; default 5s
      external-rate-limit: 5 # max concurrent in-flight requests; default 10
```

`external-skip`, `external-timeout`, and `external-rate-limit` are parsed
from the same `links:` map that MDS027 and MDS068 already use.
MDS027 and MDS068 each tolerate unknown keys from the shared block, so
MDS072 must do the same (tolerate `style`, `site-root`, etc.).

### HTTP strategy

1. For each external URL in `linkgraph.Links(f)` + autolinks:

  - Skip if the URL matches any compiled `external-skip` pattern.
  - Check the package-level `sync.Map` cache keyed by raw URL string.
  - If not cached: acquire a semaphore slot (rate limit), make an HTTP HEAD
     request with the configured timeout, release the slot, store the result.
  - On HTTP 405 (Method Not Allowed): retry with GET.

2. Non-2xx responses and transport errors are cached as failures and emitted
   as diagnostics pointing at the link node's position in `f`.
3. The `http.Client` is package-level with `CheckRedirect: nil`
   (follow redirects), `Transport: http.DefaultTransport`.

### Cache

```go
// package-level; lives for the process lifetime (CLI is short-lived)
var (
    urlCache   sync.Map            // key: string URL, value: urlResult
    semaphore  chan struct{}        // sized to external-rate-limit
    httpClient *http.Client
)
```

`urlResult` holds `{statusCode int, err error}`. Cache a successful response
once (even 4xx/5xx) so duplicate URLs across many files cost at most one
request per run. Cache transport errors too (DNS failure, timeout).

### Diagnostic message

```text
external URL returned HTTP 404: https://example.com/missing
```

or

```text
external URL unreachable: https://example.com (dial tcp: i/o timeout)
```

### Allocation budget

This rule is opt-in and network-bound. Per-run allocation cost is dominated
by HTTP I/O; the ≤ 10 allocs/op budget (plan 195) does not apply.
The `alloc_budget_test.go` integration test must **skip** MDS072 (add it to
the existing exemption list or to the `t.Skip` condition).

### Rule is NOT repo-scoped

Each file emits diagnostics for URLs it contains. If two files both link to
the same broken URL they both emit a diagnostic. `RepoScoped` would collapse
them into one; the behavior is more useful per-file (the reader sees which
file to fix). The URL *response cache* is shared, but the diagnostic sites
differ per file.

## Implementation notes

Two deviations from the design landed during implementation:

- **WASM build split.** Importing `net/http` into the shared rule grew
  the WebAssembly artifact past its 14 MiB size budget (12.5 MiB to
  19.2 MiB). The HTTP probing now lives in `probe_net.go` behind a
  `!(js && wasm)` build tag; `probe_wasm.go` carries a no-op prober. A
  browser sandbox cannot make the outbound requests anyway, so the WASM
  engine emits no MDS072 diagnostics. Config parsing and AST traversal
  stay shared, so `.mdsmith.yml` validates identically on every host.
- **Collect URLs by AST walk, not `linkgraph.Links`.** `linkgraph`
  rejects external destinations in `ParseTarget`, so it surfaces no
  http/https URLs. `Check` walks `f.AST` for `*ast.Link`, `*ast.Image`,
  and `*ast.AutoLink` nodes directly.
- **No bad fixture.** A bad fixture with a live URL would hit the
  network on every `go test` run, so only a good fixture ships; the
  HTTP paths are covered by `rule_test.go` with `httptest.NewServer`.
- **Alloc/timing gates skip the rule.** The rule is network-bound, so
  the per-rule alloc and timing gates (`alloc_budget_test.go`,
  `perrule_bench_test.go`) exclude it via an `isNetworkBound` predicate
  rather than measuring a Check that would issue a real request against
  the gate fixture's external URL.

## Review fixes (post-implementation)

A three-angle `code-review xhigh` pass found and fixed several defects
in the first implementation:

- **Enable-with-`true` was a silent no-op (critical).** The bare
  `external-link-check: true` form leaves `cfg.Settings` nil, so
  `checker.ConfigureRule` returns the rule without calling
  `ApplySettings`. The original `RateLimit==0` "unconfigured" sentinel
  therefore fired for the documented enable path and probed nothing.
  Fixed by baking the defaults into the registered instance (`newRule`)
  — `CloneInstance` and the bare-enable path both inherit them — and
  removing the sentinel. `Check` now runs whenever the engine invokes it
  (i.e. when enabled); the network-bound gates skip the rule instead.
  Regression-tested end-to-end in
  `internal/integration/externallink_enable_test.go`.
- **Autolink diagnostics anchored at (1, 1).** An `AutoLink` stores its
  text in a private value node with no walkable `*Text` child, so the
  first-text-offset walk found nothing. Fixed with an autolink-aware
  `position` that locates the literal `<url>` in the enclosing block's
  source (the technique linkstyle already uses for autolinks).
- **`external-rate-limit` capped nothing.** The semaphore was a per-Rule
  field, but the engine clones one Rule per worker, so each clone held
  its own semaphore and the global concurrency was bounded only by the
  worker count. Moved the semaphore (and the result cache) to package
  scope so the cap is global.
- **Concurrent double-probe.** The cache was check-then-act, so two
  workers hitting the same URL both missed and both issued a request. A
  `singleflight.Group` keyed by URL now collapses concurrent probes onto
  one request, restoring the "one request per URL per run" guarantee.
- **Body not drained / no keep-alive; missing `defer`.** The shared
  probe client now drains a bounded prefix of each response before Close
  (so connections return to the pool) and releases the rate-limit slot
  with `defer`, so a probe panic cannot leak a slot.
- **Images added.** External image destinations (`![alt](url)`) are now
  probed alongside links and autolinks.

Known limitations (accepted, not fixed here):

- **Process-lifetime probe state.** The result cache and the global
  rate-limit semaphore are sized/populated once and live for the
  process. That fits the short-lived CLI. A long-running native
  `mdsmith lsp` session, though, keeps a probed URL's result for its
  whole lifetime and never re-checks it, and freezes the concurrency cap
  at the first run's `external-rate-limit`. Per-run eviction and
  re-sizing are left as a future change.
- **Duplicate-autolink column.** Two identical autolinks in one block
  both anchor their diagnostic to the first occurrence's column (the URL
  is still flagged correctly and probed once). This matches
  `linkstyle.autolinkPosition`; per-occurrence bookkeeping is not worth
  it for so rare a case.

## Tasks

1. [x] Create `internal/rules/externallink/rule.go`:

  - `init()` calling `rule.Register(&Rule{})`.
  - `Rule` struct with `links ExternalLinkConfig`.
  - `ExternalLinkConfig`: `Skip []string`, `Timeout time.Duration`
     (default 5s), `RateLimit int` (default 10).
  - Compiled skip patterns cached in `Rule` after `ApplySettings`.
  - `ID() "MDS072"`, `Name() "external-link-check"`,
     `Category() "link"`, `EnabledByDefault() false`.
  - `ApplySettings` parses `links:` sub-block; tolerates unknown keys
     shared with MDS027/MDS068 (`site-root`, `validate-images`,
     `validate-reference-style`, `style`, etc.).
  - `Check(f *lint.File)` walks `linkgraph.Links(f)` plus autolinks,
     skips non-HTTP/HTTPS destinations and skip-pattern matches, then
     calls `checkURL` for each.
  - `checkURL` consults `urlCache`; on miss acquires semaphore, sends
     HTTP HEAD (retry GET on 405), caches result, releases semaphore.
  - Diagnostic message format as above.

2. [x] Initialize the package-level HTTP client and semaphore lazily
   via `sync.Once` inside the first `checkURL` call (rate limit comes
   from the rule's `RateLimit` field at the time of the first call).
   Because the rule is a singleton, the first `Check` call across all
   files fixes the effective rate limit for the run.
3. [x] Create `internal/rules/externallink/rule_test.go` with:

  - `TestCheck_SkipNonHTTP`: image `data:` URLs and local paths → no diag.
  - `TestCheck_SkipPatternMatch`: URL matching `external-skip` → no diag.
  - `TestCheck_CacheHit`: second `Check` for same URL uses cached result.
  - `TestCheck_HTTP200`: mock server returning 200 → no diag.
  - `TestCheck_HTTP404`: mock server returning 404 → diag with status code.
  - `TestCheck_HTTP405ThenGET`: mock that returns 405 on HEAD, 200 on GET
     → no diag (fallback GET succeeded).
  - `TestCheck_TransportError`: unreachable host → diag with error text.
  - `TestApplySettings_Defaults`: zero config → `Timeout=5s`, `RateLimit=10`.
  - `TestApplySettings_CustomTimeout`: `external-timeout: 2s` → `Timeout=2s`.
  - `TestApplySettings_UnknownLinksKey`: tolerated keys (site-root, style)
     → no error.

4. [x] Create fixture dirs:
   `internal/rules/MDS072-external-link-check/good/` and `bad/`.

  - `good/no-external-links.md`: file with only local links.
  - `bad/broken-url.md`: has an external URL fixture; mark in YAML
     front-matter that MDS072 is enabled and the server is mocked
     (or mark the file as requiring a skip-all override so the
     fixture runner doesn't make real network calls — see note below).
  - **Note**: fixture tests in `internal/integration/rules_test.go`
     run `Check` on real files. To avoid real HTTP calls in the bad
     fixture, add a `testSkip: true` annotation to the bad fixture's
     YAML header OR restructure the bad fixture to rely on the skip-
     pattern mechanism. Preferred: use `httptest.NewServer` in the
     integration harness if supported, else mark bad fixtures as
     `networkDependent: true` and skip in CI without `--net` flag.

5. [x] Add MDS072 to the per-rule alloc-ceiling gate so the
   allocation budget gate does not fail on a network-bound rule.
6. [x] Add `externallink` import (blank `_`) to the rules registration
   file (wherever the other rules are blank-imported, e.g.
   `internal/rules/rules.go` or `cmd/mdsmith/main.go`).
7. [x] Create `internal/rules/MDS072-external-link-check/README.md`
   following the style of adjacent rule READMEs.
8. [x] Run `go test ./...` and `go run ./cmd/mdsmith check .`.
9. [x] Rule README catalogs (`internal/rules/index.md`, the
   markdownlint-coverage page) regenerated to list MDS072.

## Acceptance Criteria

- [x] `go test ./...` green.
- [x] `go run ./cmd/mdsmith check .` reports 0 failures.
- [x] `go tool golangci-lint run` reports no issues.
- [x] MDS072 is off by default; enabling it and pointing at a file with
  a broken URL emits a diagnostic with the HTTP status code.
- [x] A URL matching `external-skip` produces no diagnostic.
- [x] The same URL referenced in two files results in exactly one HTTP
  request per run (cache hit on second call).
- [x] `external-timeout` is honored (configurable per-request timeout).
- [x] `external-rate-limit` caps in-flight requests (semaphore).
- [x] Unknown `links:` keys shared with MDS027/MDS068 are tolerated.
- [x] Fixture tests pass. The good fixture emits no diagnostics; no
  bad fixture ships, so the fixture harness makes no network call.
- [x] MDS072 stays under the alloc gates via the unconfigured early
  return; a `perRuleAllocCeiling` entry pins it at 4.
