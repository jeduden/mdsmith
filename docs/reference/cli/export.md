---
command: export
summary: Write a portable, directive-free copy of a Markdown file.
---
# `mdsmith export`

Write a portable, directive-free copy of a Markdown file
to stdout. Generated section markers are stripped and
the bodies stay as plain Markdown. `<?include?>` content
is inlined recursively. The result renders on any
Markdown tool with no mdsmith knowledge.

```text
mdsmith export [flags] <file>
```

Takes a single file argument. The source file is never
modified in any mode.

## Flags

| Flag               | Default | Description                                                  |
|--------------------|---------|--------------------------------------------------------------|
| `-c`, `--config`   | auto    | Override config path (auto-discovers)                        |
| `-o`, `--output`   | stdout  | Write output to `<path>` instead of stdout                   |
| `--max-input-size` | `2MB`   | Max file size (e.g. `2MB`, `0`=none)                         |
| `--fix`            | false   | Regenerate stale directive bodies in memory before stripping |
| `--no-check`       | false   | Skip the staleness check; export on-disk bytes as-is         |

`--fix` and `--no-check` are mutually exclusive — one
regenerates, the other trusts the file as-is. Passing
both is a usage error.

## Staleness modes

By default `export` is **faithful**: it strips markers
only after verifying each directive body equals what the
engine would generate.

| Mode         | Behavior                                                                        |
|--------------|---------------------------------------------------------------------------------|
| (default)    | Refuse on any stale body; exit non-zero with a diagnostic naming the directive. |
| `--fix`      | Regenerate stale bodies in memory, then strip markers.                          |
| `--no-check` | Skip the check; emit on-disk bytes verbatim.                                    |

The default never silently masks drift between a
directive and its rendered body. `--fix` is the opt-in
convenience for a one-shot fresh export; `--no-check`
trusts callers who already know the file is fresh.

## What gets stripped

- Opening and closing markers of every paired directive
  (`<?catalog?>`, `<?include?>`, `<?toc?>`, `<?build?>`).
  The body between them is kept verbatim — or
  regenerated first under `--fix`.
- Markerless directives with no body (for example
  `<?allow-empty-section?>` and `<?require?>`) are
  removed outright.
- Marker-like text the engine treats as literal content
  — for example inner same-type markers nested inside an
  outer directive — is left in place.

After stripping, blank lines around the removed markers
are normalised so the output is stable and passes
`mdsmith check`. Front matter is kept as-is.

Exporting an already directive-free file is a no-op:
`export` is idempotent.

## Examples

```bash
mdsmith export PLAN.md             # write portable copy to stdout
mdsmith export PLAN.md -o PLAN.export.md
mdsmith export --fix README.md     # regenerate stale bodies first
mdsmith export --no-check README.md > README.snapshot.md
```

## Exit codes

| Code | Meaning                                                                                        |
|------|------------------------------------------------------------------------------------------------|
| 0    | Export succeeded                                                                               |
| 1    | Refused: a directive body was stale in default mode                                            |
| 2    | Runtime or configuration error (missing file, conflict between `--fix` and `--no-check`, etc.) |

## See also

- [`mdsmith fix`](fix.md) — the same regeneration engine
  applied in place on disk.
- [`mdsmith check`](check.md) — read-only sibling that
  surfaces the same "out of date" diagnostic without
  attempting to export.
