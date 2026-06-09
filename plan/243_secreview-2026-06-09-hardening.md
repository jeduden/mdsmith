---
id: 243
title: 'Security hardening batch — 2026-06-09 audit'
status: "🔲"
model: sonnet
summary: >-
  Two low-risk hardening fixes from the 2026-06-09 audit:
  return an error instead of panicking in cuetemplate, and
  route convention.go YAML through the safe wrapper. Closes
  findings S002 and S003.
---
# Security hardening batch — 2026-06-09 audit

## Goal

Ship the two informational findings from the
[2026-06-09 audit](../docs/security/2026-06-09-full-repo-audit/report.md)
as one small batch. Each is independent but too small to
warrant its own plan.

## Fixes

### A. cuetemplate panics on json.Marshal (S002)

`buildCUESource` in
[`internal/cuetemplate/cuetemplate.go`](../internal/cuetemplate/cuetemplate.go)
calls `json.Marshal(emit)` on front-matter-derived data
and `panic`s on error. A catalog `row-expr:` directive
reaches it. In the LSP it is unrecovered (see plan 242)
and crashes the server. Standard go-yaml v3 scalars
cannot trigger it today. A future non-marshalable type
would make it live.

Return an error instead and propagate it up through
`Render`. The catalog caller (`renderTemplate`) already
handles render errors, so only the signature changes.

### B. convention.go bypasses the safe YAML wrapper (S003)

`parseConventionFileBody` in
[`internal/config/convention.go`](../internal/config/convention.go)
calls `yaml.Unmarshal(data, &node)` directly. It does
not use `yamlutil.UnmarshalNodeSafe`. There is no current
exploit, since a yaml.Node does not expand aliases. But
it breaks the project rule that every user-YAML entry
point goes through the safe wrapper. A later `.Decode()`
on the node would then reintroduce the alias risk.

Route the read through `yamlutil.UnmarshalNodeSafe`. The
package already imports `yamlutil` for the pre-check.

## Tasks

1. [ ] Add a failing test that drives `buildCUESource`
   down its marshal-error branch and asserts an error
   return (not a panic); thread the error through
   `Render` and its catalog caller.
2. [ ] Replace the `panic` in `buildCUESource` with an
   error return.
3. [ ] Replace `yaml.Unmarshal(data, &node)` in
   `parseConventionFileBody` with
   `yamlutil.UnmarshalNodeSafe`; keep behaviour identical
   for alias-free input and add a test that an
   anchor/alias convention file is rejected.

## Acceptance Criteria

- [ ] `buildCUESource` returns an error on marshal
      failure; no panic reaches the catalog render path or
      the LSP.
- [ ] Convention-file YAML is parsed via
      `yamlutil.UnmarshalNodeSafe`; an anchor/alias file is
      rejected.
- [ ] Both fixes covered by tests (driven red/green).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
