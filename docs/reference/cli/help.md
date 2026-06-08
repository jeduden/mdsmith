---
command: help
summary: Show built-in documentation for rules, metrics, and concept pages.
---
# `mdsmith help`

Pulls every page from the embedded docs bundle — no network
call, no extra install. Useful for piping into `AGENTS.md`,
`CLAUDE.md`, `.cursor/rules`, or any LLM context.

```text
mdsmith help <topic>
```

## Topics

| Topic                 | Description                              |
| --------------------- | ---------------------------------------- |
| `rule [id\|name]`     | Show rule documentation                  |
| `metrics [id\|name]`  | Show metric documentation                |
| `kinds`               | Show concept page for file kinds         |
| `kinds-cli`           | Summarize the `kinds` subcommand surface |
| `placeholder-grammar` | Show placeholder vocabulary reference    |
| `patterns`            | List maintainability patterns by rule    |

`mdsmith help rule` with no argument lists every rule with
its ID, name, status, and one-line description.

## Examples

```bash
mdsmith help rule line-length
mdsmith help rule MDS001
mdsmith help metrics token-estimate
mdsmith help kinds > AGENTS-context.md
mdsmith help patterns
mdsmith help patterns -f json
```

## Exit codes

| Code | Meaning                      |
| ---- | ---------------------------- |
| 0    | Topic printed                |
| 2    | Unknown topic / lookup error |
