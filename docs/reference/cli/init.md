---
command: init
summary: >-
  Write `.mdsmith.yml` from the built-in defaults, a
  `--starter` scaffold, or a `--from-markdownlint`
  conversion; add curated `.mdsmith/` bundles with `--add`;
  `--force` overwrites an existing config and `--list`
  prints every starter and pack.
---
# `mdsmith init`

Sets up mdsmith config in the current directory. It has two
independent jobs: write `.mdsmith.yml`, and scaffold
additive `.mdsmith/` packs beside it.

```text
mdsmith init [--starter <name>] [--from-markdownlint[=path]] [--apm] [--add <pack>] [--force] [--list]
```

## The config

`init` writes `.mdsmith.yml` from one source. The default
lists every rule with its built-in settings, so a rule is
easy to flip off or override with a clear diff. `--starter`
writes a workflow scaffold instead. `--from-markdownlint`
converts a peer config. The last two cannot combine.

An existing `.mdsmith.yml` is left unchanged, with a notice,
and the run still exits 0. Pass `--force` to overwrite it.

## Additive packs

`--add <pack>` scaffolds a curated bundle of `.mdsmith/`
files beside the config. It is repeatable and comma-joined,
so `--add wordlists --add stopwords` and
`--add wordlists,stopwords` both work. A pack never
overwrites an existing file, and it does not write the
config, so it pairs with any source and works on an
already-initialized project. An unknown pack name fails
before anything is written.

## Discovery

`mdsmith init --list` prints every starter and pack with a
one-line description, then exits without writing anything.

## Flags

| Flag                        | Effect                                                                 |
| --------------------------- | ---------------------------------------------------------------------- |
| `--starter=$name`           | Scaffold a ready-to-edit config for a workflow                         |
| `--from-markdownlint`       | Convert a markdownlint config found in the current directory           |
| `--from-markdownlint=$path` | Convert the markdownlint config at `$path`                             |
| `--apm`                     | Scaffold the APM kind pack and write the coexistence `ignore:` posture |
| `--add=$pack`               | Also scaffold a curated `.mdsmith/` pack (repeatable)                  |
| `--force`                   | Overwrite an existing `.mdsmith.yml` instead of skipping it            |
| `--list`                    | Print the available starters and packs, then exit                      |

## Starters

`--starter <name>` writes a hand-authored, commented
`.mdsmith.yml` tuned for one authoring workflow, instead of
the rule-by-rule defaults. Available starters:

| Name  | Scaffolds                                                  |
| ----- | ---------------------------------------------------------- |
| `okf` | [Open Knowledge Format](../../guides/okf.md) bundle config |

An unknown name fails with exit code 2 and lists the valid
names. A starter is a starting *configuration*; it is
unrelated to the `<?build?>` directive's recipe.

With `--from-markdownlint` and no `=path`, the command
probes the same file names markdownlint-cli does, in
order:

1. `.markdownlint.jsonc`
2. `.markdownlint.json`
3. `.markdownlint.yaml`
4. `.markdownlint.yml`
5. `.markdownlintrc`

Each file may hold JSON, JSONC (comments and trailing
commas), or YAML.

The converted file contains only the rules whose behavior
differs from mdsmith's defaults; every unlisted rule keeps
its default. The
[markdownlint rule mapping](../markdownlint-mapping.md)
supplies the rule correspondence. Options with no mdsmith
setting, unknown keys, tag toggles, and `extends:` are
reported as notes — on stderr and as a `# Not converted:`
comment block in the generated file. Notes do not fail the
command.

## APM coexistence

`--apm` is a shorthand for two coordinated operations:
scaffolding the APM kind pack (the same files `--add apm`
writes) and appending a coexistence `ignore:` block to
`.mdsmith.yml`. On a fresh repo, the block is written
directly. On an existing config, the block is printed to
stderr for you to merge.

Entries are scoped to detected harness directories.
A Claude-only repo gets `.claude/rules/**` but not
`.kiro/steering/**`. Compiled root files (`AGENTS.md`,
`CLAUDE.md`, `GEMINI.md`) and the APM module cache
(`apm_modules/**`) are always included.

See [Coexist with APM](../../guides/coexist-with-apm.md)
for the full ownership table and CI ordering.

## Packs

`--add <pack>` writes a curated bundle you own and edit.
Available packs:

| Name        | Scaffolds                                                             |
| ----------- | --------------------------------------------------------------------- |
| `wordlists` | The `no-llm-tells` word-lists as editable files                       |
| `apm`       | APM kind files for `.apm/` source (skill, prompt, instruction, agent) |

The `wordlists` pack writes
`.mdsmith/wordlists/ai-speak.yaml` and `ai-openers.yaml`
from the built-in [`no-llm-tells`](../conventions.md)
vocabulary. These are the same curated tell words and
sentence openers, but as editable files you own. Each
file's header shows the exact `lists:` reference. Nothing
reads a file until a rule names it.

No word-list ships compiled into the binary; this pack is
how you put the curated set on disk. An existing list file
is left untouched, so a re-run never clobbers your edits.
`init` does not edit `.mdsmith.yml` to reference the files.

The `apm` pack writes four kind files under
`.mdsmith/kinds/` that validate APM's frontmatter contracts
and enforce size limits on `.apm/` source files. It is also
applied by `--apm`, which additionally writes the
coexistence `ignore:` posture. Prefer `--apm` over
`--add apm` when setting up APM coexistence for the first
time.

## Examples

Default config:

```bash
mdsmith init
$EDITOR .mdsmith.yml
```

Scaffold an OKF bundle config:

```bash
mdsmith init --starter okf
$EDITOR .mdsmith.yml
```

Convert a markdownlint config:

```bash
mdsmith init --from-markdownlint
$EDITOR .mdsmith.yml
```

Set up APM coexistence (kind pack + ignore posture):

```bash
mdsmith init --apm
$EDITOR .mdsmith.yml   # review the ignore: block
```

Add the curated word-lists to an existing project:

```bash
mdsmith init --add wordlists
$EDITOR .mdsmith/wordlists/ai-speak.yaml
```

List everything init can produce:

```bash
mdsmith init --list
```

See
[Migrate from markdownlint](../../guides/migrate-from-markdownlint.md)
for a worked conversion, including the emitted notes.

## Exit codes

| Code | Meaning                                                                         |
| ---- | ------------------------------------------------------------------------------- |
| 0    | Config written or skipped; packs applied (conversion notes may show)            |
| 2    | Unknown starter or pack, conflicting sources, no config found, or a write error |
