---
command: list
summary: Selection-style commands that walk the workspace and emit matches.
---
# `mdsmith list`

Parent for the selection-style subcommands. Each child
walks every workspace Markdown file (filtered by
`.mdsmith.yml` `files:` / `ignore:`) and emits matches
in its own shape; the parent itself is just a router.

```text
mdsmith list <subcommand> [flags] [args]
```

## Subcommands

| Subcommand                  | Description                                                  |
|-----------------------------|--------------------------------------------------------------|
| [`query`](query.md)         | Select files by a CUE expression on front matter.            |
| [`backlinks`](backlinks.md) | List incoming links that point at a target file (or anchor). |

Run `mdsmith list <subcommand> --help` for per-command
flags, exit codes, and worked examples.
