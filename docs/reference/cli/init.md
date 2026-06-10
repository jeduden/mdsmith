---
command: init
summary: >-
  Generate a default `.mdsmith.yml` config in the current
  directory, or convert an existing markdownlint config
  with `--from-markdownlint`.
---
# `mdsmith init`

Writes `.mdsmith.yml` in the current directory. Without
flags, the file lists every rule with its built-in default
settings, so individual rules can be flipped off or
overridden with a clear diff.

```text
mdsmith init [--from-markdownlint[=path]]
```

Refuses to overwrite an existing `.mdsmith.yml`. Takes no
positional arguments.

## Flags

| Flag                        | Effect                                                       |
| --------------------------- | ------------------------------------------------------------ |
| `--from-markdownlint`       | Convert a markdownlint config found in the current directory |
| `--from-markdownlint=$path` | Convert the markdownlint config at `$path`                   |

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

Convert a markdownlint config:

```bash
mdsmith init --from-markdownlint
$EDITOR .mdsmith.yml
```

See
[Migrate from markdownlint](../../guides/migrate-from-markdownlint.md)
for a worked conversion, including the emitted notes.

## Exit codes

| Code | Meaning                                                             |
| ---- | ------------------------------------------------------------------- |
| 0    | Config written (conversion notes may still be present)              |
| 2    | `.mdsmith.yml` exists, no markdownlint config found, or parse error |
