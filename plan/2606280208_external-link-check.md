---
id: 2606280208
title: External URL link checking rule (MDS071)
status: "🔳"
summary: >-
  Add MDS071 `external-link-check` — an opt-in rule that validates external
  URLs by making HTTP HEAD (fallback GET) requests, caching results for the
  run, and reporting non-2xx responses as diagnostics.
model: opus
depends-on: [172]
---
# External URL link checking rule (MDS071)

## Goal

Add `MDS071 external-link-check`, a default-off rule that closes the gap with
gomarklint's `external-link` rule. MDS071 HEAD-checks every `http://` and
`https://` URL in the workspace, caches results per URL for the run, and
emits a diagnostic for each URL returning an error, a 4xx, or a 5xx.
Resolves [issue #47](https://github.com/jeduden/mdsmith/issues/47).

## Background

MDS027 (`cross-file-reference-integrity`) already validates local file and
heading links. External URLs are out of scope for MDS027 because they require
network I/O and are unsuitable for the hot default-on path. MDS071 is
off-by-default and opt-in per the same pattern as MDS068 (`link-style`).

`linkstyle.LinksConfig` already carries `external-skip`, a list of skip
patterns. Tests live in `rule_test.go` at line 268. MDS071 reads that key.
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
MDS071 must do the same (tolerate `style`, `site-root`, etc.).

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
The `alloc_budget_test.go` integration test must **skip** MDS071 (add it to
the existing exemption list or to the `t.Skip` condition).

### Rule is NOT repo-scoped

Each file emits diagnostics for URLs it contains. If two files both link to
the same broken URL they both emit a diagnostic. `RepoScoped` would collapse
them into one; the behavior is more useful per-file (the reader sees which
file to fix). The URL *response cache* is shared, but the diagnostic sites
differ per file.

## Tasks

1. [ ] Create `internal/rules/externallink/rule.go`:

  - `init()` calling `rule.Register(&Rule{})`.
  - `Rule` struct with `links ExternalLinkConfig`.
  - `ExternalLinkConfig`: `Skip []string`, `Timeout time.Duration`
     (default 5s), `RateLimit int` (default 10).
  - Compiled skip patterns cached in `Rule` after `ApplySettings`.
  - `ID() "MDS071"`, `Name() "external-link-check"`,
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

2. [ ] Initialize the package-level HTTP client and semaphore lazily
   via `sync.Once` inside the first `checkURL` call (rate limit comes
   from the rule's `RateLimit` field at the time of the first call).
   Because the rule is a singleton, the first `Check` call across all
   files fixes the effective rate limit for the run.
3. [ ] Create `internal/rules/externallink/rule_test.go` with:

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

4. [ ] Create fixture dirs:
   `internal/rules/MDS071-external-link-check/good/` and `bad/`.

  - `good/no-external-links.md`: file with only local links.
  - `bad/broken-url.md`: has an external URL fixture; mark in YAML
     front-matter that MDS071 is enabled and the server is mocked
     (or mark the file as requiring a skip-all override so the
     fixture runner doesn't make real network calls — see note below).
  - **Note**: fixture tests in `internal/integration/rules_test.go`
     run `Check` on real files. To avoid real HTTP calls in the bad
     fixture, add a `testSkip: true` annotation to the bad fixture's
     YAML header OR restructure the bad fixture to rely on the skip-
     pattern mechanism. Preferred: use `httptest.NewServer` in the
     integration harness if supported, else mark bad fixtures as
     `networkDependent: true` and skip in CI without `--net` flag.

5. [ ] Add MDS071 to the `alloc_budget_test.go` exemption list so the
   allocation budget gate does not fail on a network-bound rule.
6. [ ] Add `externallink` import (blank `_`) to the rules registration
   file (wherever the other rules are blank-imported, e.g.
   `internal/rules/rules.go` or `cmd/mdsmith/main.go`).
7. [ ] Create `internal/rules/MDS071-external-link-check/README.md`
   following the style of adjacent rule READMEs.
8. [ ] Run `go test ./...` and `go run ./cmd/mdsmith check .`.
9. [ ] Update `docs/reference/cli/check.md` or the rule list page if
   one exists to mention MDS071.

## Acceptance Criteria

- [ ] `go test ./...` green.
- [ ] `go run ./cmd/mdsmith check .` reports 0 failures.
- [ ] `go tool golangci-lint run` reports no issues.
- [ ] MDS071 is off by default; enabling it and pointing at a file with
  a broken URL emits a diagnostic with the HTTP status code.
- [ ] A URL matching `external-skip` produces no diagnostic.
- [ ] The same URL referenced in two files results in exactly one HTTP
  request per run (cache hit on second call).
- [ ] `external-timeout` is honored (configurable per-request timeout).
- [ ] `external-rate-limit` caps in-flight requests (semaphore).
- [ ] Unknown `links:` keys shared with MDS027/MDS068 are tolerated.
- [ ] Fixture tests pass (good fixture: no diag; bad fixture: skipped or
  mocked to avoid real network calls).
- [ ] MDS071 is exempt from the `alloc_budget_test.go` gate.
