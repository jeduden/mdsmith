---
summary: >-
  Axis 2 of the APM analysis: all 32 apm subcommands, the Markdown
  each one reads or writes, and where mdsmith intersects — today or
  as a candidate feature.
---
# APM subcommands and their Markdown surface

Axis 2 of the analysis walks every `apm` subcommand and asks two
questions: which Markdown does the subcommand read or write, and
what does mdsmith offer around that Markdown? The opportunity ids
(`A-1`, `C-2`, …) resolve in
[opportunities.md](opportunities.md).

Subcommand behavior below comes from the
[APM CLI reference](https://microsoft.github.io/apm/reference/)
(fetched 2026-07-08). Groups follow the reference layout.

## Dependency lifecycle

| Subcommand  | Markdown surface                                                                                           |
| ----------- | ---------------------------------------------------------------------------------------------------------- |
| `init`      | writes `apm.yml` only; no Markdown                                                                         |
| `install`   | deploys primitives into `.github/`, `.claude/`, `.agents/`, …; rewrites relative links into `apm_modules/` |
| `uninstall` | deletes deployed files listed in the lockfile                                                              |
| `update`    | re-resolves refs, redeploys the same Markdown surface                                                      |
| `lock`      | resolves and pins; deploys nothing                                                                         |
| `prune`     | deletes deployed files of orphaned packages                                                                |
| `outdated`  | read-only; compares locked refs to remotes                                                                 |
| `deps`      | lists / explains what sits under `apm_modules/`                                                            |
| `find`      | reverse lookup: which package deployed a given file                                                        |

`install`, `uninstall`, `update`, and `prune` all mutate committed
Markdown in bulk. Every run raises the same three questions for a
repo that lints its Markdown:

- Do the arriving third-party files pass the repo's `mdsmith
  check`, and should they have to? Deployed primitives are
  vendored content; a repo needs a deliberate `ignore:` posture
  for `.github/prompts/`, `.claude/commands/`, `.agents/skills/`,
  and friends — or a relaxed kind per directory.
- Links: deployed files point into the gitignored `apm_modules/`,
  so MDS027 (cross-file reference integrity) sees dangling targets
  on a fresh clone until `apm install` runs. `find` and `deps`
  answer provenance questions that
  [mdsmith deps](../../reference/cli/deps.md) answers for
  include/link graphs — the two graphs never join today.
- Staleness: `uninstall`/`prune` delete files that other Markdown
  may link to; nothing rewrites those incoming references.

## Compile

`apm compile` turns `.apm/instructions/*.instructions.md` into the
root context files: `AGENTS.md` (root plus per-directory scoped
files in distributed mode), `CLAUDE.md`, `GEMINI.md`, and
`.github/copilot-instructions.md`. With
`agents_md.mode: managed_section` it edits only the block between
`<!-- apm:start -->` and `<!-- apm:end -->`.

This is the deepest intersection with mdsmith, which already owns
a [generated-section model](../../background/concepts/generated-section.md)
(`<?directive?>` … `<?/directive?>`), a fix loop that keeps bodies
in sync, a git merge driver for conflicts inside generated blocks,
and rules that skip generated content. The two managed-block
grammars live in the same files — AGENTS.md and CLAUDE.md — and do
not know about each other. mdsmith's own `CLAUDE.md` is
directive-generated; a repo using both tools has two generators
writing one file.

## Prompt execution

| Subcommand | Markdown surface                                   |
| ---------- | -------------------------------------------------- |
| `list`     | none (prints `scripts:` from `apm.yml`)            |
| `run`      | compiles `*.prompt.md` with `--param` substitution |
| `preview`  | same compilation, rendered without executing       |
| `runtime`  | none (manages AI CLI binaries)                     |

`run` and `preview` substitute `${input:name}` tokens inside
prompt bodies. mdsmith's
[placeholder grammar](../../background/concepts/placeholder-grammar.md)
exists for exactly this file shape — template tokens inside
Markdown that rules must treat as opaque — but its vocabulary is
config-declared per project and does not ship an APM preset.

## Author and distribute

| Subcommand    | Markdown surface                                             |
| ------------- | ------------------------------------------------------------ |
| `pack`        | bundles `.apm/` primitives + `README.md` and other root docs |
| `publish`     | zips `apm.yml`, `.apm/`, `README.md`, `CHANGELOG.md`         |
| `unpack`      | extracts bundle Markdown into a project (deprecated)         |
| `plugin`      | scaffolds `plugin.json` + `apm.yml`; no Markdown             |
| `marketplace` | authoring side reads package `README.md` descriptions        |
| `search`      | none (queries a marketplace index)                           |
| `view`        | prints package metadata, primitive counts                    |

Everything a producer packs or publishes is authored Markdown with
contractual front matter (see
[apm-model.md](apm-model.md)). APM validates the front matter
parse (`apm compile --validate`) and scans for hidden Unicode
(`apm audit`), and stops there. Body conventions in APM's own docs
— one topic per file, bullets over prose, agent bodies under 300
lines, `SKILL.md` under 500 lines / 5000 tokens — have no checker.
mdsmith's kinds, schemas, size rules, and token budget rule are
that checker, minus a published preset for APM's file shapes.

## Governance and trust

| Subcommand       | Markdown surface                                                            |
| ---------------- | --------------------------------------------------------------------------- |
| `audit`          | scans deployed Markdown for hidden Unicode; replays install to detect drift |
| `policy`         | none (YAML policy diagnostics)                                              |
| `approve`/`deny` | none (executable-trust lists)                                               |
| `lifecycle`      | none (install-hook scripts)                                                 |

`apm audit --ci` is the natural CI neighbor of `mdsmith check .`:
one gates dependency integrity, the other content quality. They
emit different report formats (APM: text/JSON/SARIF/Markdown;
mdsmith: text/JSON) and neither references the other. A repo that
adopts both wants a documented composition — ordering, ignore
boundaries, and which tool owns which failure.

## Plumbing

`cache`, `config`, `self-update`, `targets`, `experimental`, and
`mcp` touch no project Markdown. `targets` matters indirectly: its
filesystem detection (`.claude/` present → claude target active)
is the same signal a repo's mdsmith config uses to decide which
deployed directories exist and need lint posture.
