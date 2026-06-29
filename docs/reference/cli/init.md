---
command: init
summary: >-
  Generate a default `.mdsmith.yml` config in the current
  directory, scaffold a workflow config with `--starter`,
  or convert an existing markdownlint config with
  `--from-markdownlint`.
---
# `mdsmith init`

Writes `.mdsmith.yml` in the current directory. Without
flags, the file lists every rule with its built-in default
settings, so individual rules can be flipped off or
overridden with a clear diff.

```text
mdsmith init [--starter <name>] [--from-markdownlint[=path]]
```

Refuses to overwrite an existing `.mdsmith.yml`. Takes no
positional arguments. `--starter` and `--from-markdownlint`
are mutually exclusive.

## Flags

| Flag                        | Effect                                                       |
| --------------------------- | ------------------------------------------------------------ |
| `--starter=$name`           | Scaffold a ready-to-edit config for a workflow               |
| `--from-markdownlint`       | Convert a markdownlint config found in the current directory |
| `--from-markdownlint=$path` | Convert the markdownlint config at `$path`                   |

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

See
[Migrate from markdownlint](../../guides/migrate-from-markdownlint.md)
for a worked conversion, including the emitted notes.

## Exit codes

| Code | Meaning                                                                                                 |
| ---- | ------------------------------------------------------------------------------------------------------- |
| 0    | Config written (conversion notes may still be present)                                                  |
| 2    | `.mdsmith.yml` exists, unknown starter, conflicting flags, no markdownlint config found, or parse error |
