---
command: metrics
summary: List, rank, and get shared Markdown metrics (file length, token estimate, readability, …).
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
| `get`      | Get all metrics for a single file      |
| `list`     | List available metrics in the registry |
| `rank`     | Rank files by selected metrics         |

## `metrics get`

```text
mdsmith metrics get [flags] <file>
```

Computes all registered file-scope metrics for a single Markdown
file and emits them as a single object (not an array).

| Flag               | Default | Description                          |
| ------------------ | ------- | ------------------------------------ |
| `-c`, `--config`   | auto    | Override config path                 |
| `-f`, `--format`   | `text`  | `text`, `json`, or `yaml`            |
| `--max-input-size` | `2MB`   | Max file size (e.g. `2MB`, `0`=none) |

Exactly one positional file argument is required. Zero or two or
more arguments are a usage error.

`metrics get` emits every registered metric including `readability`,
`sentences`, and `avg-words-per-sentence` — there is no metric
selector. Narrow the output downstream with `jq` or `yq`.

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

The readability metrics `readability`, `sentences`, and
`avg-words-per-sentence` are opt-in. They do not appear in
the default rank table. Request them with `--metrics`:

```bash
mdsmith metrics rank --metrics readability --by readability docs/
```

## Built-in metrics

| ID     | Name                     | Default | Description                                                  |
| ------ | ------------------------ | ------- | ------------------------------------------------------------ |
| MET001 | `bytes`                  | yes     | File size measured in bytes                                  |
| MET002 | `lines`                  | yes     | Total non-virtual line count                                 |
| MET003 | `words`                  | yes     | Word count from extracted plain text                         |
| MET004 | `headings`               | yes     | Heading count                                                |
| MET005 | `token-estimate`         | yes     | Estimated token count (0.75 tokens per word)                 |
| MET006 | `conciseness`            | yes     | Heuristic conciseness score (0–100, lower is less concise)   |
| MET008 | `readability`            | no      | Automated Readability Index; higher is harder to read        |
| MET009 | `sentences`              | no      | Sentence count from extracted plain text                     |
| MET010 | `avg-words-per-sentence` | no      | Average words per sentence; zero when there are no sentences |

## Examples

```bash
# Get all metrics for one file as JSON
mdsmith metrics get -f json README.md

# Get all metrics as YAML and extract readability with yq
mdsmith metrics get -f yaml docs/guide.md | yq .readability

# List all metrics
mdsmith metrics list

# Rank by bytes, show top 10
mdsmith metrics rank --by bytes --top 10 .

# Rank docs by readability (opt-in metric)
mdsmith metrics rank --metrics readability --by readability docs/

# Rank with multiple metrics including sentences
mdsmith metrics rank --metrics bytes,sentences --by sentences plan/

# YAML output
mdsmith metrics rank -f yaml --by words docs/
```

## Exit codes

| Code | Meaning                |
| ---- | ---------------------- |
| 0    | Output produced        |
| 2    | Runtime / config error |
