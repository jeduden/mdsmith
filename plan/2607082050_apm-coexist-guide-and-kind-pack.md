---
id: 2607082050
title: "APM coexistence: `mdsmith init --apm`, guide, and kind pack"
status: "✅"
model: sonnet
summary: >-
  Add an `mdsmith init --apm` template that scaffolds the APM kind
  pack (`.mdsmith/kinds/apm-*`) plus the `ignore:`/`overrides:`
  posture that keeps `mdsmith fix` off APM's hash-pinned deployed
  files, and document it in a coexist guide. Opportunities C-2, A-1,
  A-2, L-1.
depends-on: [2607082048]
---
# APM coexistence template and guide

## Goal

Let one command set up a repo that runs both mdsmith
and APM. `mdsmith init --apm` scaffolds the kind pack
that lints the `.apm/` sources and the
`ignore:`/`overrides:` posture that keeps `mdsmith
fix` off APM's hash-pinned deployed files. A guide
documents what the template writes and why.

## Background

[APM](https://github.com/microsoft/apm) deploys
third-party Markdown into committed paths
(`.github/prompts/`, `.claude/rules/`,
`.agents/skills/`, and more) and pins a SHA-256 per
file in `apm.lock.yaml`. A repo-wide `mdsmith fix`
rewrites those bytes and trips `apm audit --ci`. The
same files follow their package's conventions, not
the consumer's, so `mdsmith check` fails on files the
team cannot edit. Both problems are laid out in the
[APM workflows analysis](../docs/research/apm-mdsmith/workflows.md)
and catalogued as C-2, L-1, A-1, and A-2 in the
[opportunity catalogue](../docs/research/apm-mdsmith/opportunities.md).

The `.apm/` source tree is the opposite case: it is
the author's own Markdown, with contractual front
matter APM's docs state but never enforce. mdsmith
kinds are the checker — the pattern the repo already
runs for its own `skill` and `plan` kinds.

Both halves are expressible in config today. But no
command assembles them, so every team rediscovers the
boundary by hand.
[`mdsmith init`](../docs/reference/cli/init.md) already
scaffolds config two ways. `--from-markdownlint`
converts a peer config. `--wordlists` writes the
curated `.mdsmith/wordlists/` files, additively, and
never clobbers an existing setup. `--apm` follows the
`--wordlists` model.

## Non-Goals

- Foreign managed-region support inside the compiled
  files. That is
  [plan 2607082049](2607082049_foreign-managed-regions.md);
  the template `ignore:`s the compiled root files
  until it lands.
- Schema grammar changes. Closed frontmatter and
  filename agreement are
  [plan 2607082051](2607082051_apm-schema-extensions.md);
  the kind pack uses only today's grammar.
- Running APM, or editing `apm.yml`. The template
  writes mdsmith config only.

## Tasks

1. Add `--apm` to
   [`mdsmith init`](../docs/reference/cli/init.md),
   modeled on `--wordlists`: additive, refuses to
   clobber, works on an already-initialized project.
2. Have `--apm` write the kind pack as
   `.mdsmith/kinds/` files — `apm-skill`,
   `apm-prompt`, `apm-instruction`, `apm-agent` —
   each with `path-pattern` and an inline frontmatter
   schema matching the
   [primitive contracts](../docs/research/apm-mdsmith/apm-model.md),
   plus per-kind `max-file-length` and `token-budget`
   (500 lines / 5000 tokens for `SKILL.md`, 300 lines
   for agents). The `apm-prompt` kind opts its content
   rules into the
   [`${input:name}` token](2607082048_apm-input-placeholder-token.md).
3. Have `--apm` write the coexistence posture: the
   `ignore:` list naming APM's deploy directories and
   `overrides:` disabling fix-capable rules on the
   compiled root files (`AGENTS.md`, `CLAUDE.md`,
   `GEMINI.md`, `.github/copilot-instructions.md`).
   On a fresh repo it lands in a new `.mdsmith.yml`;
   on an existing one it prints the block to merge,
   never rewriting the file — the `--wordlists` rule.
4. Scope the `ignore:` set to the harness directories
   actually present, reusing the filesystem detection
   `apm targets` uses (`.claude/` present → claude
   dirs), so the config names only real paths.
5. Write `docs/guides/coexist-with-apm.md`: the
   ownership table (APM-deployed, APM-compiled,
   `.apm/` source, hand-authored), what `--apm`
   writes, and the `apm audit --ci` + `mdsmith check`
   CI ordering. Follow the
   [Prettier guide](../docs/guides/coexist-with-prettier.md)
   and
   [Vale + remark guide](../docs/guides/coexist-with-vale-and-remark.md).
6. Add the guide to the guides catalog, cross-link the
   research README, and document `--apm` in the init
   reference and its `--wordlists` neighbor.
7. Verify end to end: run `mdsmith init --apm` in a
   scratch `.apm/` fixture and confirm `mdsmith check`
   passes on a conformant tree and flags a missing
   `description`.
8. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [x] `mdsmith init --apm` writes the `apm-*` kind
      files and the coexistence posture, and refuses
      to clobber an existing `.mdsmith.yml`.
- [x] After `mdsmith init --apm`, `mdsmith fix` does
      not rewrite a file under `.github/prompts/`.
- [x] The kind pack flags a `.apm/skills/x/SKILL.md`
      missing `name` or `description`.
- [x] The kind pack flags an `.apm/agents/x.agent.md`
      body over 300 lines.
- [x] The `ignore:` set names only harness
      directories present in the repo.
- [x] `docs/guides/coexist-with-apm.md` exists, is
      reachable from the guides catalog and research
      README, and the init reference lists `--apm`.
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint
      run` reports no issues.
- [x] `mdsmith check .` — 0 failures.
