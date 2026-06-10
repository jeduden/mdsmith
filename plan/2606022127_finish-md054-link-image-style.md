---
id: 2606022127
title: Finish MD054 link-image-style coverage in MDS068
status: "✅"
model: sonnet
depends-on: [172]
summary: >-
  Extend MDS068 link-style from a partial, inline-vs-
  reference cover of markdownlint MD054 to the full
  six-style link/image policy, so the coverage matrix can
  drop the partial marker and mdsmith's markdownlint
  coverage is complete.
---
# Finish MD054 link-image-style coverage in MDS068

## Goal

Make [MDS068 link-style](../internal/rules/MDS068-link-style/README.md)
a full implementation of markdownlint
[MD054][md054] (link-image-style). That closes the last
markdownlint rule gap. The
[coverage matrix](../docs/research/markdownlint-coverage/README.md)
can then drop the `partial:` marker on the MDS068 → MD054
row.

## Background

MD054 governs which link and image *forms* a document may
use. It has six independent toggles: autolink
(`<https://x>`), inline (`[t](u)`), full reference
(`[t][label]`), collapsed (`[t][]`), shortcut (`[t]`), and
inline image. Each is allowed or forbidden on its own.

MDS068 today carries a `links.style.form` axis with three
values only: `inline`, `reference`, and `any`. It folds
full, collapsed, and shortcut into one `reference` bucket,
never inspects autolinks, and skips images outright. The
rule README and the
[coverage matrix](../docs/research/markdownlint-coverage/README.md)
both mark the MD054 cover `partial: true`, and the rule is
opt-in.

One nuance shapes the scope. markdownlint ships MD054 with
all six toggles set to "allowed", so on defaults the rule
emits nothing. The gap is therefore about *rule coverage*,
not benchmark speed: the
[parity convention](../docs/reference/conventions.md) does
not need MD054 enabled to match rumdl's runtime, because
default MD054 is a no-op. Completing it lets mdsmith *claim*
full MD054, which the
[markdown-linters comparison](../docs/background/markdown-linters.md)
states is the only outstanding markdownlint rule.

The migration guide
([migrate-from-markdownlint](../docs/guides/migrate-from-markdownlint.md))
also lists MD054 as not-yet-covered; that row updates here.

## Tasks

1. Add an MD054-shaped axis to MDS068, e.g.
   `links.style.link-image-style`, accepting the six
   markdownlint toggle names (`autolink`, `inline`,
   `full`, `collapsed`, `shortcut`, `inline-image`) with
   allow/forbid booleans. Default every toggle to allowed
   so an enabled-but-unconfigured rule is a no-op, matching
   markdownlint. Write the failing settings-parse test
   first.
2. Classify each link and image node from the goldmark AST
   into exactly one of the six styles. Resolve reference
   sub-forms (full vs collapsed vs shortcut) from the
   node's label and reference table. Add images, which the
   rule skips today.
3. Emit one diagnostic per offending node, naming the
   forbidden style and the allowed set, close enough to
   MD054's wording that a migrating user recognises it.
4. Keep the existing `path`, `extension`, and `form` axes
   working and independent. Decide whether `form` becomes a
   thin alias over the new axis or stays separate; document
   the choice next to the settings handler.
5. Add `good/` and `bad/` fixtures for each style under
   the MDS068 rule directory, plus unit tests in
   `rule_test.go` covering every toggle red/green.
6. Drop `partial: true` from the `markdownlint` and `rumdl`
   MD054 blocks in the MDS068 README front matter, then
   regenerate the matrix with
   `mdsmith-release sync-coverage-matrix`.
7. Update prose: the MDS068 README settings table, the
   [migration guide](../docs/guides/migrate-from-markdownlint.md)
   MD054 row, and the "only MD054 outstanding" lines in the
   [markdown-linters comparison](../docs/background/markdown-linters.md)
   (Future Plans + Structural Linting).

## Acceptance Criteria

- [x] MDS068 classifies all six MD054 styles for both links
      and images, with a per-style allow/forbid toggle.
- [x] An enabled-but-unconfigured MD054 axis emits no
      diagnostics (matches markdownlint defaults).
- [x] The MDS068 README no longer marks MD054 `partial`,
      and `sync-coverage-matrix --check` is clean.
- [x] The
      [markdown-linters comparison](../docs/background/markdown-linters.md)
      no longer lists MD054 as outstanding.
- [x] Per-style `good/`/`bad/` fixtures and unit tests
      cover every toggle.
- [x] `mdsmith check .` passes.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues

[md054]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md054.md
