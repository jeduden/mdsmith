---
title: Coexist with APM
weight: 55
summary: >-
  Keep mdsmith and APM (Microsoft's Agent Package Manager) from
  stepping on each other: ignore APM-deployed files so mdsmith fix
  never breaks a content hash, and validate your .apm/ source tree
  with the APM kind pack.
---
# Coexist with APM

[APM](https://github.com/microsoft/apm) (Microsoft's Agent Package
Manager) deploys Markdown into committed paths and pins a SHA-256
per file in `apm.lock.yaml`. Running `mdsmith fix` on those files
breaks the hash and trips `apm audit --ci`. The same deployed files
follow the source package's conventions, not the consumer's, so
`mdsmith check` may flag content the team cannot edit.

The `.apm/` source tree is the opposite case: it is the author's own
Markdown, with contractual frontmatter APM's docs state but never
enforce. mdsmith kind files are the right checker for it.

One command sets up both sides:

```bash
mdsmith init --apm
```

## What `--apm` writes

`--apm` has two effects:

1. **Kind pack**: scaffolds four `.mdsmith/kinds/apm-*.yaml` files
   that validate `.apm/` source files against APM's frontmatter
   contracts and size limits.

2. **Coexistence posture**: appends an `ignore:` block to
   `.mdsmith.yml` (on a fresh repo) that scopes `mdsmith fix` away
   from APM-deployed and APM-compiled files. On an existing config it
   prints the block to merge by hand.

The `--apm` flag detects which harness directories are present
(`.github/`, `.claude/`, `.agents/`, `.windsurf/`, `.kiro/`,
`.cursor/`) and names only those in the `ignore:` list. A repo that
only uses GitHub Copilot gets `.github/prompts/**` and
`.github/instructions/**`; a Claude Code repo also gets
`.claude/rules/**`.

## Which tool owns what

| File or directory                     | Owner                | `mdsmith fix`? |
| ------------------------------------- | -------------------- | -------------- |
| `.apm/skills/*/SKILL.md`              | APM author (you)     | yes            |
| `.apm/prompts/*.prompt.md`            | APM author (you)     | yes            |
| `.apm/instructions/*.instructions.md` | APM author (you)     | yes            |
| `.apm/agents/*.agent.md`              | APM author (you)     | yes            |
| `.github/prompts/**`                  | APM (deployed)       | no — ignored   |
| `.github/instructions/**`             | APM (deployed)       | no — ignored   |
| `.claude/rules/**`                    | APM (deployed)       | no — ignored   |
| `.agents/skills/**`                   | APM (deployed)       | no — ignored   |
| `.windsurf/rules/**`                  | APM (deployed)       | no — ignored   |
| `.kiro/steering/**`                   | APM (deployed)       | no — ignored   |
| `.cursor/rules/**`                    | APM (deployed)       | no — ignored   |
| `apm_modules/**`                      | APM (cache)          | no — ignored   |
| `AGENTS.md`, `CLAUDE.md`, `GEMINI.md` | APM (compiled roots) | no — ignored   |
| `.github/copilot-instructions.md`     | APM (compiled root)  | no — ignored   |

## The kind pack

The four kinds validate APM's frontmatter contracts at edit time:

| Kind              | Path pattern                          | Required fields       | Size limits         |
| ----------------- | ------------------------------------- | --------------------- | ------------------- |
| `apm-skill`       | `.apm/skills/*/SKILL.md`              | `name`, `description` | 500 lines, 5000 tok |
| `apm-prompt`      | `.apm/prompts/*.prompt.md`            | `description`         | none                |
| `apm-instruction` | `.apm/instructions/*.instructions.md` | `description`         | none                |
| `apm-agent`       | `.apm/agents/*.agent.md`              | `name`, `description` | 300 lines           |

Optional frontmatter fields are allowed on each kind:

- `apm-prompt`: `input`, `allowed-tools`, `model`, `argument-hint`
- `apm-instruction`: `applyTo`
- `apm-agent`: `model`, `color`

The `apm-prompt` kind also opts all content rules into the
`apm-input-token` placeholder, so `${input:name}` parameter
references in prompt bodies are treated as opaque rather than
flagged as prose violations.

## Quick start

Run on a fresh repo or an existing project:

```bash
mdsmith init --apm
```

On a fresh repo, a new `.mdsmith.yml` is created with the `ignore:`
posture and the four kind files are scaffolded beside it. On an
existing project, the kind files are added and the posture block is
printed to stderr for you to paste in.

Combine with other packs or starters:

```bash
# APM kind pack plus the curated no-llm-tells word-lists
mdsmith init --apm --add wordlists

# APM kind pack on top of an OKF starter
mdsmith init --starter okf --apm
```

`--apm` does not touch `apm.yml`, `apm.lock.yaml`, or any APM
subcommand. It writes mdsmith config only.

## CI ordering

Run `apm audit --ci` before `mdsmith check` so the integrity gate
catches a hash drift before the content gate reports style issues:

```yaml
- name: apm audit
  run: apm audit --ci

- name: mdsmith check
  run: mdsmith check .
```

Both jobs can run in parallel if the repo has no deployed files that
`mdsmith check` must see (because they are in `ignore:`).

## Adding harness dirs later

If you later add a new harness (for example, you start using Kiro
and `.kiro/` appears), re-run `mdsmith init --apm` with an existing
config to get the updated merge hint for the new `ignore:` entries.

## See also

- [File kinds](file-kinds.md) — how `path-pattern` assigns files to
  kinds.
- [Metrics trade-offs](metrics-tradeoffs.md) — tuning the 500-line
  and 5000-token caps for skill files.
- [APM research](../research/apm-mdsmith/README.md) — the full
  analysis of mdsmith and APM workflows.
- [Init reference](../reference/cli/init.md) — all `mdsmith init`
  flags.
</content>
