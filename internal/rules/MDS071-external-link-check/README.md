---
id: MDS071
name: external-link-check
status: ready
description: Probe external http and https URLs and flag any that returns a transport error or a 4xx/5xx response.
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
# MDS071: external-link-check

Probe external http and https URLs and flag any that returns a transport error or a 4xx/5xx response.

This rule closes the gap with gomarklint's `external-link` check
(issue #47). It is off by default and opt-in, like MDS068
(link-style). Network I/O has no place on the default `mdsmith check`
hot path. It reads the shared `links:` config block — the same block
MDS027 and MDS068 read. So `external-skip`, `external-timeout`, and
`external-rate-limit` sit beside `site-root` and `style` per kind.

The rule checks inline links (`[text](url)`) and autolinks
(`<https://example.com>`). It does not run in the WebAssembly engine.
The browser sandbox forbids the outbound requests it needs. So the
Obsidian plugin and other WASM hosts emit no MDS071 diagnostics.

## Settings

| Setting                     | Type   | Default | Description                                        |
| --------------------------- | ------ | ------- | -------------------------------------------------- |
| `links.external-skip`       | list   | `[]`    | Regex patterns; a matching URL is not probed       |
| `links.external-timeout`    | string | `"5s"`  | Per-request timeout as a Go duration               |
| `links.external-rate-limit` | int    | `10`    | Maximum concurrent in-flight requests; minimum `1` |

Each external URL is probed once per run with an HTTP HEAD request. A
URL whose HEAD returns 405 (Method Not Allowed) is retried with GET.
Redirects are followed; a final 2xx or 3xx passes. Results are cached
per URL for the run, so the same URL across many files costs one
request. A non-positive `external-timeout` falls back to `5s`; a
rate limit below `1` clamps to `1`.

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

- **ID**: MDS071
- **Name**: `external-link-check`
- **Status**: ready
- **Default**: disabled, opt-in. Network I/O keeps it off the hot path.
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: link
- **gomarklint**: [external-link][gomarklint-rules]

[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
