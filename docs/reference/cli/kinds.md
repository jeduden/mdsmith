---
command: kinds
summary: Inspect declared file kinds and resolve effective rule config per file.
---
# `mdsmith kinds`

See [File Kinds](../../guides/file-kinds.md) for the concept.
Each subcommand resolves the layered config (defaults →
convention → kind → overrides) for one file path or kind name
and prints the merged result.

```text
mdsmith kinds <subcommand> [args]
```

Each subcommand accepts `--json` for stable structured
output.

## Subcommands

| Subcommand          | Description                                        |
| ------------------- | -------------------------------------------------- |
| `list`              | Print declared kinds with their merged bodies      |
| `show <name>`       | Print one kind's merged body                       |
| `path <name>`       | Print resolved schema path of `required-structure` |
| `resolve <file>`    | Resolved kind list and per-leaf provenance summary |
| `why <file> <rule>` | Full per-rule merge chain, including no-op layers  |

## JSON schemas

`kinds list` → `{"kinds": [<body>...]}`; `show <name>` →
one body. Body: `{"name", "rules", "categories"}` where
`rules[<name>]` follows the YAML rule-cfg union (`false`,
`true`, or a settings map).

`kinds resolve <file>` returns `{file, kinds, categories,
rules}`. Each rule entry is `{final, leaves}` with one
leaf per `enabled` and `settings.<key>`.

`kinds why <file> <rule>` adds two arrays. `layers[]`
lists every applicable layer in chain order; no-op layers
carry `"set": false` and omit `value`. `leaves[].chain`
records the layers that set each leaf, in chain order:

```json
{"file": "plan/9_big.md", "rule": "max-file-length",
 "final": {"max": 900},
 "layers": [
   {"source": "default", "set": true, "value": {"max": 300}},
   {"source": "kinds.plan", "set": true, "value": {"max": 500}},
   {"source": "overrides[0]", "set": true, "value": {"max": 900}}],
 "leaves": [{"path": "settings.max", "value": 900,
   "source": "overrides[0]", "chain": [
     {"source": "default", "value": 300},
     {"source": "kinds.plan", "value": 500},
     {"source": "overrides[0]", "value": 900}]}]}
```

Source labels: `default`, `front-matter override`,
`front-matter`, `kind-assignment[<i>]`, `kinds.<name>`,
or `overrides[<i>]`.

## Examples

```bash
mdsmith kinds list
mdsmith kinds show plan
mdsmith kinds path plan
mdsmith kinds resolve plan/9_big.md
mdsmith kinds why plan/9_big.md max-file-length --json
```

## Exit codes

| Code | Meaning                                |
| ---- | -------------------------------------- |
| 0    | Output produced                        |
| 2    | Unknown kind, unresolved schema, error |
