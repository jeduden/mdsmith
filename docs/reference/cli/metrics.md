---
command: metrics
summary: List and rank shared Markdown metrics (file length, token estimate, readability, …).
---
# `mdsmith metrics`

Metrics are the shared measurements rules compute: file
length, section length, token estimate, readability scores.
This CLI surfaces them as a standalone tool for triage.

```text
mdsmith metrics <command> [flags] [files...]
```

## Subcommands

| Subcommand | Description                            |
| ---------- | -------------------------------------- |
| `list`     | List available metrics in the registry |
| `rank`     | Rank files by selected metrics         |

## `metrics list`

```text
mdsmith metrics list [flags]
```

| Flag             | Default | Description                |
| ---------------- | ------- | -------------------------- |
| `-f`, `--format` | `text`  | `text` or `json`           |
| `--scope`        | `file`  | Metric scope (only `file`) |

## `metrics rank`

```text
mdsmith metrics rank [flags] [files...]
```

| Flag                | Default | Description                           |
| ------------------- | ------- | ------------------------------------- |
| `-c`, `--config`    | auto    | Override config path                  |
| `-f`, `--format`    | `text`  | `text` or `json`                      |
| `--metrics`         | —       | Comma-separated metric IDs to compute |
| `--by`              | —       | Metric ID to rank by                  |
| `--order`           | `desc`  | `asc` or `desc`                       |
| `--top`             | `0`     | Limit output to N rows (`0` = all)    |
| `--no-gitignore`    | false   | Skip gitignore filtering              |
| `--follow-symlinks` | config  | Follow symlinks; tri-state            |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)  |

`metrics rank` counts only **authored bytes**. Content
between `<?include?>` and `<?catalog?>` markers is
excluded. Embedded content is measured against its source
file, not the host that pulls it in.

With no file arguments, defaults to the current directory.

## Examples

```bash
mdsmith metrics list
mdsmith metrics rank --by bytes --top 10 .
mdsmith metrics rank --by token-estimate --top 5 docs/
mdsmith metrics rank --metrics bytes,sentences --by sentences plan/
```

## Exit codes

| Code | Meaning                |
| ---- | ---------------------- |
| 0    | Output produced        |
| 2    | Runtime / config error |
