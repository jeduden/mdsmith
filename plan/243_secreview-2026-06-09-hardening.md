---
id: 243
title: 'Security hardening batch — 2026-06-09 audit'
status: "✅"
model: sonnet
summary: >-
  Two low-risk hardening fixes from the 2026-06-09 audit:
  return an error instead of panicking in cuetemplate, and
  guard the convention pre-check with the yamlutil alias
  rejection. Closes findings S002 and S003.
---
# Security hardening batch — 2026-06-09 audit

## Goal

Ship the two informational findings from the
[2026-06-09 audit](../docs/security/2026-06-09-full-repo-audit/report.md)
as one small batch. Each is independent but too small to
warrant its own plan.

## Fixes

### A. cuetemplate panics on json.Marshal (S002)

`buildSource` in
[`internal/cuetemplate/cuetemplate.go`](../internal/cuetemplate/cuetemplate.go)
calls `json.Marshal(emit)` on front-matter-derived data
and `panic`s on error. A catalog `row-expr:` directive
reaches it. In the LSP it is unrecovered (see plan 242)
and crashes the server. The audit rated the branch
unreachable. It is live today: yaml.v3 decodes the
scalars `.inf`, `-.inf`, and `.nan` into float64
±Inf/NaN values, and `json.Marshal` rejects those.
Front matter like `weight: .inf` in any file matched
by a row-expr catalog triggers it.

Return an error instead and propagate it up through
`Render`. The catalog caller (`renderTemplate`) already
handles render errors, so only the signature changes.

### B. convention.go bypasses the safe YAML wrapper (S003)

`validateConventionScalar` in
[`internal/config/convention.go`](../internal/config/convention.go)
— the pre-check on the `.mdsmith.yml` top-level
`convention:` scalar — calls `yaml.Unmarshal(data,
&node)` directly with no anchor/alias guard. There is
no current exploit, since a yaml.Node does not expand
aliases. But it breaks the project rule that every
user-YAML entry point rejects anchors/aliases. A later
`.Decode()` on the node would then reintroduce the
alias risk.

Guard the read with `yamlutil.RejectYAMLAliases`. The
kind-file and convention-file loaders run the same
pre-check. The tolerant `yaml.Unmarshal` stays: its
parse errors defer to Load's `UnmarshalSafe`.

### C. Same-class sweep

Fix A's discarded-error pattern recurs in
`frontmatterExpr`
([`internal/schema/parse_inline.go`](../internal/schema/parse_inline.go)):
the primitive branch dropped the `json.Marshal` error
as "unreachable". The same `.inf`/`.nan` front matter
reaches it through inline kind schemas and `proto.md`.
The branch now propagates the error. The catalog
row-expr render error also names the matched file, so
one bad file in a large catalog is findable.

## Tasks

1. [x] Add a failing test that drives `buildSource`
   down its marshal-error branch and asserts an error
   return (not a panic); thread the error through
   `Render` and its catalog caller.
2. [x] Replace the `panic` in `buildSource` with an
   error return.
3. [x] Guard `validateConventionScalar` with
   `yamlutil.RejectYAMLAliases`; keep behaviour
   identical for alias-free input and add a test that
   anchor/alias config YAML is rejected.
4. [x] Sweep the same discarded-error pattern:
   propagate the `json.Marshal` error in
   `frontmatterExpr` and name the matched file in the
   catalog row-expr render error.

## Acceptance Criteria

- [x] `buildSource` returns an error on marshal
      failure; no panic reaches the catalog render path or
      the LSP.
- [x] The `.mdsmith.yml` `convention:` scalar pre-check
      rejects anchor/alias YAML via
      `yamlutil.RejectYAMLAliases`.
- [x] All fixes covered by tests (driven red/green).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
