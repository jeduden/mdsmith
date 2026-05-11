---
command: list query
summary: Select Markdown files by a CUE expression on front matter.
---
# `mdsmith list query`

Print paths of Markdown files whose front matter satisfies
a CUE expression.

```text
mdsmith list query [flags] <cue-expr> [files...]
```

With no file arguments, searches the current directory
recursively. Files without front matter are skipped (use
`--verbose` to see the reasons on stderr).

## Flags

| Flag               | Default | Description                          |
|--------------------|---------|--------------------------------------|
| `-c`, `--config`   | auto    | Override config path                 |
| `-0`, `--null`     | false   | NUL-delimit output (for `xargs -0`)  |
| `-v`, `--verbose`  | false   | Print skipped files on stderr        |
| `--max-input-size` | `2MB`   | Max file size (e.g. `2MB`, `0`=none) |

## Examples

```bash
mdsmith list query 'status: "✅"' plan/
mdsmith list query '#mdsxx & {status: "ready"}' internal/rules/
mdsmith list query -0 'kinds: [..."plan"...]' . | xargs -0 wc -l
```

The expression is full CUE — match scalars, regex, list
membership, optional fields, and structural shapes.

## Exit codes

| Code | Meaning             |
|------|---------------------|
| 0    | At least one match  |
| 1    | No matches          |
| 2    | Runtime/parse error |
