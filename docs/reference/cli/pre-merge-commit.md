---
command: pre-merge-commit
summary: Install / manage a pre-merge-commit hook that runs `mdsmith fix` after a merge.
---
# `mdsmith pre-merge-commit`

Manage a Git `pre-merge-commit` hook that runs
`mdsmith fix .` after Git resolves every per-file merge
(and the [merge driver](merge-driver.md) regenerates
`<?catalog?>`/`<?include?>` blocks) but before the merge
commit is created. Modified `.md` / `.markdown` files are
re-staged automatically.

```text
mdsmith pre-merge-commit <subcommand> [args]
```

## Subcommands

### `install`

Install the hook at `.git/hooks/pre-merge-commit`, or at
the path configured by `core.hooksPath`. The hook walks
the worktree using `.mdsmith.yml` `ignore:`.

The hook's scope is a superset of the merge driver's:
`.gitattributes` generation skips `ignore:` entries that
can't be represented (`!` negation patterns, patterns
with whitespace), so the hook may run `mdsmith fix` on
files the [merge driver](merge-driver.md) wasn't
registered for.

Explicit file lists are not accepted. Scope the hook by
editing `.mdsmith.yml` `ignore:` instead.

### `uninstall`

Remove the hook if it was installed by mdsmith. Refuses
to remove user-authored hooks.

### `status`

Show whether the hook is installed and whether the
installed script matches the canonical glob-based
template.

## Examples

```bash
mdsmith pre-merge-commit install
mdsmith pre-merge-commit status
mdsmith pre-merge-commit uninstall
```

## See also

- [`mdsmith merge-driver`](merge-driver.md) — runs first,
  per-file, before this hook
