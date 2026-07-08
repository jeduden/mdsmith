---
id: 2607082049
title: "Foreign managed-region protection for `mdsmith fix`"
status: "🔲"
model: opus
summary: >-
  Add a `foreign-regions:` config listing `{start, end}` marker
  pairs (e.g. APM's `<!-- apm:start -->`/`<!-- apm:end -->`) that
  the fix pipeline treats as opaque and style rules skip, while
  whole-file rules still count the bytes. Opportunity C-1.
depends-on: []
---
# Foreign managed-region protection

## Goal

Make `mdsmith fix` and the style rules safe inside a
region another generator owns. APM is the first case.
Its `managed_section` mode writes a block bounded by
`<!-- apm:start -->` and `<!-- apm:end -->`. A repo
must be able to declare that block so mdsmith never
rewrites its bytes.

## Background

APM's `managed_section` compile mode writes agent
context into a block bounded by `<!-- apm:start -->`
and `<!-- apm:end -->` and pins a SHA-256 of the
file in `apm.lock.yaml`. The sanctioned migration
path wraps a hand-written `AGENTS.md` with those
markers.

mdsmith's
[generated-section engine](../docs/background/concepts/generated-section.md)
recognizes only its own `<?directive?>` markers. It
has no concept of a foreign generator's region, so
`mdsmith fix` reflows the bytes inside an APM block
(table alignment, trailing-space trimming, optional
reflow). That changes the bytes `apm audit --ci`
verifies and trips its drift check. This is
opportunity C-1 in the
[APM opportunity catalogue](../docs/research/apm-mdsmith/opportunities.md);
the collision is documented in the
[workflows analysis](../docs/research/apm-mdsmith/workflows.md).

The generated-section engine is the template here. It
already keeps fixers out of directive bodies. Whole-file
metrics still count those bytes.

## Non-Goals

- Regenerating the foreign region. mdsmith treats it
  as opaque; the owning tool regenerates it.
- Resolving merge conflicts inside a foreign region.
  The
  [merge driver](../docs/reference/cli/merge-driver.md)
  stays scoped to mdsmith's own directive blocks.
- Auto-detecting markers. Regions are declared in
  config, not inferred.

## Tasks

1. Red/green: config parse tests for a top-level
   `foreign-regions:` key — a list of
   `{start, end}` string pairs, glob-scopable through
   [`overrides:`](../docs/reference/globs.md).
2. Red/green: region-scanner tests that locate
   matched marker pairs in a file and report byte
   ranges; cover zero markers, a single unmatched
   marker, and nested/duplicate pairs.
3. Wire the fix pipeline to treat bytes inside a
   matched region as opaque — no fixer rewrites them
   — reusing the exclusion path the generated-section
   engine already uses.
4. Make style and content rules skip diagnostics
   inside a region, while whole-file rules
   ([MDS022](../internal/rules/MDS022-max-file-length/README.md),
   [MDS028](../internal/rules/MDS028-token-budget/README.md))
   still count the bytes.
5. Add a check that diagnoses a malformed region
   (start without end, or a duplicated marker), since
   APM requires each marker exactly once.
6. Write the reference doc for `foreign-regions:` and
   link it from the
   [coexist-with-APM guide](2607082050_apm-coexist-guide-and-kind-pack.md).
7. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [ ] `mdsmith fix` leaves bytes between a declared
      `{start, end}` pair unchanged, including
      otherwise-fixable trailing spaces and table
      misalignment.
- [ ] A style-rule violation inside the region emits
      no diagnostic; the same violation outside it
      still does.
- [ ] MDS022 and MDS028 still count the region's
      bytes toward file length and token budget.
- [ ] A start marker with no matching end marker
      produces a diagnostic.
- [ ] The region config is glob-scopable via
      `overrides:`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint
      run` reports no issues.
- [ ] `mdsmith check .` — 0 failures.
