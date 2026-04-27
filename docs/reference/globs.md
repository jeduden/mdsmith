---
summary: >-
  Glob pattern syntax across mdsmith config, directives,
  and CLI argument expansion, with the supported
  exclusion semantics for each surface.
---
# Glob patterns

mdsmith uses globs in three places. The supported syntax
and the exclusion semantics differ at each one. This page
documents the current behavior; the long-term goal is to
unify on a single matcher and a single naming convention,
but that's a separate effort — until it lands, the table
below is the source of truth.

## Surfaces and matchers

| Surface                  | Matcher                | Field name      | `!`-exclusion |
|--------------------------|------------------------|-----------------|---------------|
| `ignore:`                | `gobwas/glob`          | list of strings | yes           |
| `overrides:.files`       | `gobwas/glob`          | `files:`        | yes           |
| `kind-assignment:.files` | `gobwas/glob`          | `files:`        | yes           |
| `<?catalog?>`            | `doublestar`           | `glob:`         | yes           |
| CLI argument expansion   | stdlib `filepath.Glob` | positional      | no            |

## Config globs (`ignore:`, `overrides:`, `kind-assignment:`)

A pattern matches a file if it matches any of:

- the raw path as given (`docs/foo.md`),
- the cleaned path (`docs/./foo.md` → `docs/foo.md`), or
- the basename (`foo.md`).

Basename matching means a pattern like `CHANGELOG.md` (no
slash) matches `CHANGELOG.md` in any directory.

### Exclusion with `!`-prefix

A pattern prefixed with `!` is an exclusion pattern. The
list matches a file when at least one non-negated pattern
matches and no exclusion pattern matches. The order of
include and exclude entries does not matter — exclusion
always wins.

```yaml
overrides:
  - files: ["docs/security/*.md", "!docs/security/proto.md"]
    rules:
      max-file-length:
        max: 1000
```

A list containing only exclusion patterns matches
nothing.

### Limits

- No `**` recursive matching unless the underlying
  `gobwas/glob` syntax supports it for the specific
  pattern; prefer explicit subdirectories.
- No brace expansion (`{a,b}`).
- A pattern that fails to compile is silently skipped.

## Directive globs (`<?catalog?>`)

`<?catalog?>` accepts a `glob:` parameter that is split
on whitespace/newlines into a list. The matcher is
[`doublestar`](https://github.com/bmatcuk/doublestar),
which supports `**`, brace expansion, and other
extensions on top of the standard glob syntax.

`!`-prefix exclusion works the same way as in config:
include patterns gather candidates, exclude patterns
remove from the result.

```markdown
<?catalog
glob:
  - "plan/*.md"
  - "!plan/proto.md"
?>
```

The directive requires at least one non-negated include
pattern; a glob list of only exclusions is rejected at
lint time.

## CLI argument expansion

Positional arguments to `mdsmith check` and `mdsmith fix`
are expanded with the standard library's `filepath.Glob`.
That matcher does not support `**`, brace expansion, or
`!`-prefix exclusion. Use shell expansion (or wrap with
`find`) for anything beyond a single-level pattern.

```bash
mdsmith check 'plan/*.md'   # works
mdsmith check '**/*.md'     # falls through as a literal
                            # path on most shells
```

For repeatable scoping, prefer `ignore:` /
`kind-assignment:` in `.mdsmith.yml` over CLI globs.

## Future work

The split between `gobwas/glob`, `doublestar`, and
`filepath.Glob`, and between the `files:` and `glob:`
field names, is a known source of confusion. A future
plan will pick one matcher (likely `doublestar`) and
one field name, migrate the three call sites, and
deprecate the displaced one.
