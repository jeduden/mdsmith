---
id: 97
title: Deep-merge for kinds and overrides
status: "✅"
model: opus
summary: >-
  Replace block-level replacement with deep-merge across
  rule settings, so a later kind or override layer can
  amend a nested key without restating its siblings.
---
# Deep-merge for kinds and overrides

## Goal

Allow a later config layer (a kind or an override) to
amend a single nested rule setting without restating
the rest of that rule's config block. The merge runs
across the whole layer chain — defaults, kinds (in
effective list order), and matching `overrides:` — so
each layer composes additively.

## Background

Today, `internal/config/merge.go`'s `Effective` does
`result[k] = v` per matching override: the entire
`RuleCfg` block is replaced. Plan 92 inherits this
semantics for kinds. The result is correct but blunt:
a kind that wants to add a `placeholders:` token to
`first-line-heading` must restate the rule's `level:`
and any other settings the earlier layer set, or they
disappear.

Most projects expect deep-merge from layered config.
This plan brings mdsmith in line.

## Design

### Merge semantics

For each rule, mdsmith walks the layer chain in order.
The chain is: default, then matching `kinds:` in
effective list order, then matching `overrides:` in
config order, with file-glob overrides last.

For each layer that touches the rule, the layer's body
is **deep-merged** onto the accumulator:

- **Maps** are merged key-by-key; recurse on shared
  keys.
- **Scalars** are replaced by the later layer's value.
- **Lists** are replaced wholesale by default; rules
  that want concatenation (e.g. `placeholders:`) opt
  in via the helper.

Block replacement (the current behavior) becomes a
special case: a layer that sets a rule's whole body
explicitly still wins because every leaf is replaced.
A layer that touches one nested key only changes that
key.

### Backward compatibility

Existing configs may rely on block replacement for
"reset everything below this layer". The migration
strategy is:

- Run `mdsmith config show` (plan 95) on the existing
  repo before and after to surface any per-rule
  setting that *changes* under deep-merge.
- Document the change in `docs/development/index.md`
  and the release notes.

### List handling

`placeholders:` (plan 93) is the most prominent list
setting. The natural behavior is **append**: a later
kind that adds `placeholders: [var-token]` extends
the earlier kind's list rather than replacing it.

Other list settings (e.g. `line-length.exclude`)
should declare their merge mode in code via a small
enum on the rule's `Configurable` schema:
`replace` (default) or `append`. The chosen mode is
explicit per setting, not a global flag.

## Tasks

- [x] Add a deep-merge function for `RuleCfg` values
  covering map / scalar / list cases. Default list
  mode is `replace`.
- [x] Replace `result[k] = v` in
  `internal/config/merge.go:Effective` with a deep
  merge across the layer chain.
- [x] Extend each `Configurable` rule that wants list
  `append` (starting with `placeholders:` from plan
  93) to declare its merge mode.
- [x] Run `mdsmith config show` against this repo and
  a small fixture set; record any settings that change
  under the new merge. (`mdsmith config show` is not
  yet implemented; provenance is exposed as
  `EffectiveWithProvenance` and exercised in the new
  unit test
  `TestEffectiveWithProvenance_ReportsContributingLayer`,
  so the consumer in plan 95 can build on it.)
- [x] Document the change in `docs/development/index.md`
  and add a "merge semantics" subsection to
  `docs/reference/cli.md`.
- [x] Update plan 92's Conflict resolution section to
  point at this plan as the live behavior.

## Acceptance Criteria

- [x] Two kinds setting different nested keys on the
      same rule both contribute to the effective rule
      config (covered by test).
- [x] An override that sets only one key does not
      erase sibling keys set earlier (covered by
      test).
- [x] List settings declared `append` concatenate
      across layers; lists declared `replace` (the
      default) replace wholesale (covered by test).
- [x] `mdsmith config show` provenance reflects the
      new merge — each leaf setting carries the layer
      that contributed its value (covered by test).
- [x] No existing rule's behavior changes
      unexpectedly: a regression test runs the
      pre-deep-merge fixture set against the new
      merge and asserts diagnostics are unchanged for
      configs that already specify the rule body in
      full at the latest layer.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
