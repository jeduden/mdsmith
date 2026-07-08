---
summary: >-
  Axis 1 of the APM analysis: the 29 user workflows the APM docs
  describe, each one's Markdown surface, and the five workflow
  clusters where mdsmith changes the outcome.
---
# APM workflows and where mdsmith fits

Axis 1 of the analysis walks the user workflows the
[APM docs](https://microsoft.github.io/apm/) describe — consumer,
producer, and enterprise ramps — and asks what mdsmith changes in
each. The short answer: five workflow clusters carry nearly all of
the intersection, because they are the ones that create, mutate,
or gate committed Markdown. The opportunity ids (`A-1`, `C-2`, …)
resolve in [opportunities.md](opportunities.md).

## Inventory

Workflows with no project-Markdown surface are listed for
completeness and not analyzed further: installing or updating the
`apm` CLI itself, managing runtime CLI binaries (`apm runtime`),
user-scope global installs (outside the repo), REST-registry
operation, SBOM export, multi-host authentication, lifecycle
scripts, executable-trust approval (`apm approve`/`deny`), and
policy authoring (YAML, not Markdown).

The remaining workflows group into five clusters:

| Cluster               | Workflows                                                                                      |
| --------------------- | ---------------------------------------------------------------------------------------------- |
| author a package      | author primitives under `.apm/`; maintain and evolve a package; preview and validate           |
| compile agent context | compile `AGENTS.md` and per-harness root files; multi-harness targeting                        |
| consume packages      | install; clone-and-install onboarding; update, inspect, remove; monorepos and virtual packages |
| distribute            | pack a bundle; publish via git; operate a marketplace                                          |
| gate in CI            | CI/CD integration; security audit and drift detection; migrate from ad-hoc setups              |

## Author a package

A producer's package is a tree of Markdown files with contractual
front matter (the exact contracts are in
[apm-model.md](apm-model.md)): `SKILL.md` with `name` +
`description`, `*.prompt.md` with five allowed keys,
`*.instructions.md` with `description` + `applyTo`, `*.agent.md`
with persona fields. APM's own docs add body conventions — one
topic per file, bullets over prose, agents under 300 lines,
`SKILL.md` under 500 lines and 5000 tokens, prompt inputs matching
`[A-Za-z][\w-]{0,63}` — and provide no checker for any of them.

Everything in that paragraph maps onto an mdsmith kind: a
`path-pattern` per primitive type, an inline schema for the front
matter, `max-file-length` and `token-budget` per kind, and the
placeholder vocabulary for `${input:name}` tokens so content rules
treat them as opaque. mdsmith already runs this exact pattern on
its own repo — the `skill` kind validates `SKILL.md` files against
a proto schema, and the `plan` kind gates `plan/*.md` front matter.
What is missing is a published preset, so every APM producer today
would have to hand-write the kinds.

The pre-release sequence APM recommends (`compile --validate` →
`compile --dry-run` → `view` → `outdated` → `audit` → `pack`) has
an obvious sixth read-only step: `mdsmith check .apm/`.

## Compile agent context

`apm compile` generates `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and
per-harness rule directories. In `managed_section` mode it owns
only the block between `<!-- apm:start -->` and `<!-- apm:end -->`
and preserves everything outside. Migrating teams wrap their
hand-written `AGENTS.md` with those markers.

mdsmith's generated-section model lives in the same files. A repo
like mdsmith's own — whose `CLAUDE.md` body is `<?catalog?>` and
`<?include?>` output — that also adopted APM would have two
generators writing one file, each blind to the other's markers.
Questions a shared user hits immediately:

- Does `mdsmith fix` reflow or edit content inside the APM block?
  (Nothing marks it as foreign; rules and fixers see plain
  Markdown and an HTML comment.)
- Does the mdsmith merge driver handle conflicts inside
  `<!-- apm:start -->` blocks? (No — it resolves conflicts inside
  `<?directive?>` blocks only.)
- Who wins when both tools' fix loops run in one pre-commit hook?

The compiled root files are also where token budgets bite: every
installed package appends instructions, and no tool reports what
the assembled `AGENTS.md` costs. mdsmith's `token-budget` rule and
`mdsmith metrics` already measure exactly this per file.

## Consume packages

`apm install` drops third-party Markdown into committed paths
(`.github/prompts/`, `.claude/commands/`, `.agents/skills/`, …)
and pins a SHA-256 per deployed file in `apm.lock.yaml`. Two
mechanical conflicts follow for any repo that also runs mdsmith:

- **Fix-vs-hash.** `mdsmith fix .` reformats every Markdown file
  it walks. Reformatting a deployed file changes its hash, so the
  next `apm audit --ci` reports drift and the next `apm install`
  warns before cleanup. The two tools' invariants collide unless
  the deployed globs are excluded from fixing.
- **Check-vs-vendored.** Third-party primitives were written to
  their author's conventions, not the consumer repo's `.mdsmith.yml`.
  Without an `ignore:` entry or a relaxed kind per deployed
  directory, `mdsmith check .` fails on files the team cannot
  edit.

Link integrity adds a third wrinkle: deployed primitives carry
links rewritten into the gitignored `apm_modules/`, valid only
after an install. MDS027 walks and flags them on a fresh clone.

None of this needs new machinery to resolve — `ignore:`,
overrides, and kinds express the posture — but the posture is
subtle enough that it needs to ship as documentation or a preset,
not be rediscovered per repo.

## Distribute

`apm pack` and `apm publish` bundle `apm.yml`, the `.apm/` tree,
and the root docs — `README.md` is the marketplace listing page.
A package's README quality is its storefront; mdsmith's content
rules plus the repo's own docs-quality stack (readability,
anti-slop conventions, link integrity) apply as-is. The
marketplace `packages[]` descriptions in `apm.yml` are YAML and
out of mdsmith's lane.

## Gate in CI

APM's CI story is `apm install --frozen` plus `apm audit --ci`
(eight baseline checks, SARIF output, GitHub rulesets). mdsmith's
CI story is `mdsmith check .` via the GitHub Action. They gate
different failure classes — dependency integrity vs. content
quality — and compose cleanly if ordered install → audit → check,
with the fix-vs-hash exclusion above in place. Neither tool's docs
mention the other today; the composition is unwritten.

Migration is the workflow where the pairing is strongest: a team
moving scattered instruction files into `.apm/` is doing a bulk
Markdown restructuring — exactly what `mdsmith check`, kinds with
`path-pattern`, and MDS033 (directory structure) are built to
police, and what the repo's own
[markdown-audit skill](../../../.claude/skills/markdown-audit/SKILL.md)
automates.
