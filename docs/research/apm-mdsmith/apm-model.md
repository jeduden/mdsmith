---
summary: >-
  APM's package model as it looks to a Markdown linter: the manifest
  and lockfile, the seven primitive types and their frontmatter
  contracts, the compile targets, and the validation surface APM
  ships today.
---
# APM's package model

[APM](https://github.com/microsoft/apm) (Agent Package Manager) is
Microsoft's open-source dependency manager for AI agent context.
A project declares skills, prompts, instructions, agents, MCP
servers, and LSP servers in one `apm.yml`; `apm install` reproduces
the same agent setup across GitHub Copilot, Claude Code, Cursor,
OpenCode, Codex, Gemini, Windsurf, and Kiro. A lockfile
(`apm.lock.yaml`) pins every dependency to a commit SHA and per-file
content hashes, and an `apm-policy.yml` lets an organization
allow-list sources, scopes, and primitive types.

The part that matters for mdsmith: almost everything APM manages is
a Markdown file with YAML front matter, and most of those files are
**committed to the consumer's repository**. APM is a package manager
whose payload is lintable prose.

## Personas and lifecycle

APM's docs split users into three ramps:

- **Consumer** — adds packages to a repo: `apm init`, `apm install`,
  `apm update`, `apm run`. Commits `apm.yml`, `apm.lock.yaml`, and
  the deployed files under `.github/`, `.claude/`, `.agents/`, etc.
  `apm_modules/` is the gitignored cache.
- **Producer** — authors a package: writes primitives under `.apm/`,
  then `apm compile` → `apm preview` → `apm pack` → publish to a
  marketplace or serve straight from the git repo.
- **Enterprise** — governs at scale: `apm-policy.yml`, `apm audit
  --ci` as a merge gate, GitHub rulesets, registry proxies.

The lifecycle concept page names five stages: `init` → `install` →
`compile` → `run` → `audit`, with audit feeding back into install
when drift is found.

## Files on disk

| File / dir                | Format         | Committed | Role                                  |
| ------------------------- | -------------- | --------- | ------------------------------------- |
| `apm.yml`                 | YAML           | yes       | manifest: deps, targets, scripts      |
| `apm.lock.yaml`           | YAML           | yes       | pinned SHAs + per-file content hashes |
| `apm_modules/`            | mixed          | no        | package cache, rebuilt from lockfile  |
| `.apm/`                   | Markdown, JSON | yes       | producer-authored primitives          |
| `.github/`, `.claude/`, … | Markdown       | yes       | deployed primitives per harness       |
| `AGENTS.md` / `CLAUDE.md` | Markdown       | yes       | compiled root context                 |
| `apm-policy.yml`          | YAML           | yes       | org allow-lists (sources, primitives) |

Two facts follow. First, a consumer repo accumulates third-party
Markdown that its own linters walk. Second, the compiled context
files sit in exactly the spot where a repo's hand-written agent
docs already live.

## Primitive types and their contracts

Producers author primitives under `.apm/` as "a markdown file (or
directory containing a primary markdown file) with frontmatter
declaring its name and its trigger conditions". The contracts are
concrete:

### `.apm/prompts/<name>.prompt.md`

The compiler preserves exactly five front-matter keys and drops the
rest with diagnostics:

| Field           | Required | Notes                                       |
| --------------- | -------- | ------------------------------------------- |
| `description`   | yes      | one line, shown in command pickers          |
| `input`         | no       | argument names; regex `[A-Za-z][\w-]{0,63}` |
| `allowed-tools` | no       | honored by Claude/Cursor only               |
| `model`         | no       | preferred model id                          |
| `argument-hint` | no       | derived from `input` when omitted           |

The body references inputs as `${input:name}`; the compiler rewrites
the token per target (Claude gets `$name`, Gemini gets TOML). The
file basename becomes the slash-command name on every harness.

### `.apm/instructions/<name>.instructions.md`

Front matter carries `description` plus `applyTo` — a glob (or
comma-separated globs) binding the rule to files. An instruction
without `applyTo` loses its scope and folds into the root context
file instead. Deployment rewrites the key per harness: Claude gets
`paths:`, Cursor gets `globs:` in an `.mdc` file, Windsurf gets
`trigger: glob`.

### `.apm/agents/<name>.agent.md`

Front matter: `name`, `description` (required), `model`, `tools`
(a mapping, not a list), `color`, `handoffs`. The docs tell authors
to keep the body under 300 lines, open with role and scope in two
sentences, and list expected outputs.

### `.apm/skills/<name>/SKILL.md`

The [agentskills.io](https://agentskills.io) format: `name`
(lowercase alphanumeric with hyphens, 1–64 chars, must equal the
directory name) and `description` (imperative, under 1024 chars).
The docs cap guidance at 500 lines / 5000 tokens for `SKILL.md` and
push overflow into `references/<topic>.md` files loaded on demand.
Optional subdirectories: `scripts/`, `references/`, `assets/`,
`examples/`.

### The rest

Hooks are JSON (`.apm/hooks/*.json`); commands ship as prompts; MCP
servers and LSP servers are `apm.yml` entries, not files. Five
package layouts exist (`.apm/` package, root `SKILL.md` bundle,
`skills/<name>/SKILL.md` collection, hooks-only, Claude
`plugin.json` collection).

## Compilation

`apm compile` reads instructions primitives and writes root context
files. Two strategies:

- **single-file** — one `AGENTS.md` (plus `CLAUDE.md` for Claude,
  `GEMINI.md` for Gemini) holding every unconditional instruction.
- **distributed** — per-harness rule directories
  (`.github/instructions/`, `.claude/rules/`, `.cursor/rules/`,
  `.windsurf/rules/`, `.kiro/steering/`), with `applyTo` scoping
  preserved.

A `compilation.agents_md.mode: managed_section` setting confines
APM's output to a marker-delimited block:

```markdown
<!-- apm:start -->
<!-- apm will insert content here -->
<!-- apm:end -->
```

Content outside the markers survives every compile run. The markers
must appear exactly once. `compile` covers only instructions — a
changed prompt, skill, agent, or hook redeploys via `apm install`,
not `apm compile`.

## Link handling

`apm install` rewrites relative links in deployed prompts,
instructions, agents, and commands so they point back into
`apm_modules/<owner>/<pkg>/…`. The rewrite applies only when the
target exists and stays inside the package root. Cross-package
links are unsupported; packages are "independent deployment units".
Skills skip the rewrite because the whole skill directory deploys
as a bundle.

The consequence: a consumer repo's committed Markdown contains
links into a gitignored directory, and those links are only valid
after `apm install` has run.

## What APM validates today

- `apm compile --validate` — strict parse of primitive front matter
  and structure.
- `apm install` — hidden-Unicode scan (bidi overrides, tag
  characters, zero-width), content-hash pinning, policy gates,
  trust prompts for transitive MCP servers.
- `apm audit` — the same Unicode scan on demand, `--strip`
  remediation, and in `--ci` mode eight baseline checks: lockfile
  presence, reference consistency, deployed-file existence, no
  orphaned packages, skill-subset consistency, MCP config
  consistency, per-file SHA-256 hash drift, and `includes` consent.
  Output formats: text, JSON, SARIF, Markdown.
- `apm outdated` — locked refs vs. remote versions.

What APM does **not** validate: Markdown quality of any kind. No
formatting or style checks, no heading or structure rules beyond
front-matter parsing, no link integrity beyond the install-time
rewrite, no size or token budgets (the SKILL.md caps are prose
guidance, not checks), no prose-quality rules. The producer docs
carry body conventions — "lead with bullet points", "one topic per
file", "keep under 300 lines" — that nothing enforces.

## Sources

- [APM docs](https://microsoft.github.io/apm/) — quickstart,
  producer ramp, manifest schema, package types, CLI reference,
  lifecycle, drift detection (fetched 2026-07-08).
- [microsoft/apm README](https://github.com/microsoft/apm).
