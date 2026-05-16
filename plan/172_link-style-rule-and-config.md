---
id: 172
title: Link-style rule and shared links config
status: "🔲"
model: opus
depends-on: [170, 171]
summary: >-
  Add an opt-in link-style rule that flags deviation
  from a per-kind declared relative/absolute,
  extension, and inline/reference policy, and finish
  the shared `links:` parser including the
  external-skip list that issue #47's rule reuses.
---
# Link-style rule and shared links config

## Goal

Give projects one switch for a consistent link style
per file kind. This closes gap G8 from the
[link handling audit](../docs/research/links/README.md).
Also land the remaining `links:` config keys. The
proposed external-link rule (issue #47) then has a
config foundation.

## Background

See the audit's corpus census in
[docs/research/links/README.md](../docs/research/links/README.md).
It found `docs/` is ~50% reference-style while rule
READMEs are ~0%. Relative, absolute, `.md`, and
extensionless targets are mixed in the same doc set.
G8's decision is a new opt-in rule that reads a shared
`links:` block. G13's decision reuses that block's
`external-skip` list.

Plan 171 introduces the block's resolution keys. This
plan adds the `style:` and `external-skip:` keys. It
also adds the rule that consumes `style:`.

## Tasks

1. Extend the `links:` parser from plan 171 with
   `style: {path, extension, form}` and
   `external-skip: []`, deep-merged per kind.
2. Add opt-in rule `link-style` (next free MDS id).
   Red tests: an absolute target under
   `style.path: relative`; a `.md` suffix under
   `extension: strip`; a reference link under
   `form: inline`.
3. Per-kind override path: a `rule-readme` kind pinned
   to `form: inline` flags a ref-style link there while
   a `docs` kind on `form: any` does not.
4. Document the rule README and link it from the audit
   doc and the rule catalog.
5. Confirm `external-skip` parses and is exposed for
   the future issue #47 rule (no HTTP code here).

## Acceptance Criteria

- [ ] `link-style` is opt-in (off by default) and
  flags path/extension/form deviations per kind.
- [ ] Per-kind override changes the verdict for the
  same link in two kinds.
- [ ] `links.external-skip` parses and is readable by
  rule code.
- [ ] Rule README exists and is in the rule catalog.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no issues.
