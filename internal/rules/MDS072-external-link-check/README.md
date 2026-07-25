---
id: MDS072
name: external-link-check
status: ready
description: Probe external http and https URLs; flag any returning a transport error or 4xx/5xx response.
category: link
nature: content
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint:
  - id: external-link
    name: external-link
    partial: false
    default: false
---
# MDS072: external-link-check

Probe external http and https URLs; flag any returning a transport error or 4xx/5xx response.

This rule closes the gap with gomarklint's `external-link` check
(issue #47). It is off by default and opt-in, like MDS068
(link-style). Network I/O has no place on the default `mdsmith check`
hot path. It reads the shared `links:` config block — the same block
MDS027 and MDS068 read. So `external-skip`, `external-timeout`, and
`external-rate-limit` sit beside `site-root` and `style` per kind.

The rule checks inline links (`[text](url)`), autolinks
(`<https://example.com>`), and images (`![alt](url)`). It probes over
the network only on native builds. The WebAssembly engine cannot reach
the network, so it treats every URL as not-probed and emits no MDS072
diagnostics. It never reports a URL as healthy on faith. A future host
bridge will let a WASM host such as the Obsidian plugin supply real
probe results.

## Settings

| Setting                         | Type   | Default | Description                                                   |
| ------------------------------- | ------ | ------- | ------------------------------------------------------------- |
| `links.external-skip`           | list   | `[]`    | Regex patterns; a matching URL is not probed                  |
| `links.external-timeout`        | string | `"5s"`  | Per-request timeout as a Go duration                          |
| `links.external-rate-limit`     | int    | `10`    | Maximum concurrent in-flight requests; minimum `1`            |
| `links.external-allow-internal` | bool   | `false` | Allow probing loopback, private, link-local, and metadata IPs |
| `links.external-max-probes`     | int    | `1000`  | Maximum distinct URLs probed per run; `0` means unlimited     |

Each external URL is probed once per run with an HTTP HEAD request. A
URL whose HEAD returns 405 (Method Not Allowed) is retried with GET.
Redirects are followed; a final 2xx or 3xx passes. Results are cached
per URL for the run, so the same URL across many files costs one
request. A non-positive `external-timeout` falls back to `5s`; a
rate limit below `1` clamps to `1`.

## SSRF guard

When `links.external-allow-internal` is `false` (the default), the
rule refuses to connect to any IP in a restricted range:

- **Loopback**: `127.0.0.0/8`, `::1/128` (via `ip.IsLoopback()`)
- **Private (RFC1918)**: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
- **Link-local**: `169.254.0.0/16` (covers AWS/GCP/Azure metadata at
  `169.254.169.254`), `fe80::/10`
- **ULA**: `fc00::/7`
- **CGN shared-address space**: `100.64.0.0/10` (covers Alibaba Cloud
  metadata at `100.100.100.200`)

The guard fires on both the initial connection and every redirect hop,
so a redirect bounce from an allowed URL to an internal endpoint is
also blocked. Redirects to restricted IP literals are caught at the
HTTP layer before the TCP dial.

A document link to a restricted address yields a `"external URL
unreachable"` diagnostic. The guard denies the connection rather than
issuing a false pass. Set `links.external-allow-internal: true` only
when you deliberately lint an internal site from a trusted network.

## Egress ceiling

`links.external-max-probes` (default 1000) caps the total number of
distinct URLs probed per run. Once the ceiling is reached, further
URLs are not probed and each is reported as `"external URL not probed:
per-run limit reached"`. This bounds egress from a run over a document
with a very large number of external links. Set to `0` for no ceiling.

## Config

Enable with defaults (5s timeout, 10 concurrent requests):

```yaml
rules:
  external-link-check: true
```

Skip intranet and example hosts, tighten the timeout, cap concurrency:

```yaml
rules:
  external-link-check:
    links:
      external-skip:
        - "^https?://localhost"
        - "^https?://127\\."
      external-timeout: 10s
      external-rate-limit: 5
```

Disable:

```yaml
rules:
  external-link-check: false
```

## Examples

### Good -- no external URLs to probe

<?include
file: good/no-external-links.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# No External Links

This file links to a [sibling document](good/no-external-links.md). It also
links to an [in-page anchor](#no-external-links). Neither is an external
URL. So the rule finds nothing to probe. It reports no diagnostics.
```

<?/include?>

The fixture suite omits a bad example on purpose. A fixture with a live
broken URL would hit the network on every `go test` run. The HTTP
behaviour lives in `rule_test.go`. That test drives a local
`httptest.NewServer`. It covers the 200, 404, 405-then-GET,
transport-error, and cache-hit paths.

## Diagnostics

| Condition                         | Message                                     |
| --------------------------------- | ------------------------------------------- |
| URL returns 4xx or 5xx            | `external URL returned HTTP <code>: <url>`  |
| URL unreachable (transport error) | `external URL unreachable: <url> (<error>)` |

## See also

- [MDS027 cross-file-reference-integrity](../MDS027-cross-file-reference-integrity/README.md)
  — validates local file and heading links; shares the `links:` block.
- [MDS068 link-style](../MDS068-link-style/README.md)
  — enforces link path, extension, and form style; shares `links:`.

## Meta-Information

- **ID**: MDS072
- **Name**: `external-link-check`
- **Status**: ready
- **Default**: disabled, opt-in. Network I/O keeps it off the hot path.
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: link
- **gomarklint**: [external-link][gomarklint-rules]

[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
