---
command: metrics
summary: Get, list, and rank shared Markdown metrics (file length, token estimate, readability, …).
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
| `get`      | Emit all metrics for a single file     |
| `list`     | List available metrics in the registry |
| `rank`     | Rank files by selected metrics         |

## `metrics get`

Emit every registered metric for a single Markdown file as a
data object. The shape is a flat map with `path` plus one key
per metric. To narrow to one field, pipe through `jq` or `yq`.

```text
mdsmith metrics get [flags] <file>
```

| Flag             | Default | Description               |
| ---------------- | ------- | ------------------------- |
| `-f`, `--format` | `text`  | `text`, `json`, or `yaml` |

```bash
mdsmith metrics get docs/guides/install.md
mdsmith metrics get -f json README.md
mdsmith metrics get -f yaml README.md | yq .readability
mdsmith metrics get -f json README.md | jq .avg-words-per-sentence
```

## `metrics list`

```text
mdsmith metrics list [flags]
```

| Flag             | Default | Description                |
| ---------------- | ------- | -------------------------- |
| `-f`, `--format` | `text`  | `text`, `json`, or `yaml`  |
| `--scope`        | `file`  | Metric scope (only `file`) |

## `metrics rank`

```text
mdsmith metrics rank [flags] [files...]
```

| Flag                | Default | Description                           |
| ------------------- | ------- | ------------------------------------- |
| `-c`, `--config`    | auto    | Override config path                  |
| `-f`, `--format`    | `text`  | `text`, `json`, or `yaml`             |
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

## Available metrics

Three metrics are off by default in `rank`: `readability`
(MET008), `sentences` (MET009), and
`avg-words-per-sentence` (MET010). They appear in `get` and
`list` unconditionally.

| Metric                   | Default | Description                                   |
| ------------------------ | ------- | --------------------------------------------- |
| `bytes`                  | yes     | Raw file byte count                           |
| `lines`                  | yes     | Total line count                              |
| `words`                  | yes     | Word count from plain text                    |
| `headings`               | yes     | Number of headings                            |
| `token-estimate`         | yes     | Token estimate (0.75 × words)                 |
| `conciseness`            | yes     | Heuristic conciseness score (0–100)           |
| `readability`            | no      | Automated Readability Index (ARI grade level) |
| `sentences`              | no      | Sentence count from plain text                |
| `avg-words-per-sentence` | no      | Average words per sentence                    |

## Examples

```bash
mdsmith metrics list
mdsmith metrics get -f json README.md
mdsmith metrics get -f yaml docs/guides/install.md
mdsmith metrics rank --by bytes --top 10 .
mdsmith metrics rank --by token-estimate --top 5 docs/
mdsmith metrics rank --metrics bytes,sentences --by sentences plan/
mdsmith metrics rank --metrics readability --by readability --top 20 docs/
```

## Exit codes

| Code | Meaning                |
| ---- | ---------------------- |
| 0    | Output produced        |
| 2    | Runtime / config error |
