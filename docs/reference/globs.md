---
summary: >-
  Glob pattern syntax across mdsmith config, directives,
  and CLI argument expansion, with the supported
  exclusion semantics for each surface.
---
# Glob patterns

mdsmith uses [`doublestar`](https://github.com/bmatcuk/doublestar) as
its single glob matcher across all surfaces. These include config
(`ignore:`, `overrides:`, `kind-assignment:`), the `<?catalog?>`
directive, and CLI argument expansion. The pattern syntax
is identical on every surface; `!`-exclusion is supported
on config and directive surfaces but not on CLI arguments.

## Surfaces

| Surface                 | Field name      | `!`-exclusion |
|-------------------------|-----------------|---------------|
| `ignore:`               | list of strings | yes           |
| `overrides:.glob`       | `glob:`         | yes           |
| `kind-assignment:.glob` | `glob:`         | yes           |
| `<?catalog?>`           | `glob:`         | yes           |
| CLI argument expansion  | positional      | no            |

## Supported syntax

All config and directive surfaces use `doublestar`, which
supports:

- `*` — any sequence of characters, no path separator
- `**` — any sequence of characters, including path separators
  (recursive match)
- `?` — any single character
- `[abc]` — character class
- `{a,b}` — brace expansion (matches either `a` or `b`)

On config and CLI surfaces, a pattern matches a file if
it matches any of:

- the raw path as given (`docs/foo.md`),
- the cleaned path (`docs/./foo.md` → `docs/foo.md`), or
- the basename (`foo.md`).

Basename matching means a pattern like `CHANGELOG.md` (no
slash) matches `CHANGELOG.md` in any directory.

Note: the `<?catalog?>` directive uses `doublestar.Glob`
for include patterns. It matches full relative paths only.
A bare filename like `CHANGELOG.md` finds only files in
the catalog file's directory, not nested paths.

## Exclusion with `!`-prefix

A pattern prefixed with `!` is an exclusion pattern. The
list matches a file when at least one non-negated pattern
matches and no exclusion pattern matches. The order of
include and exclude entries does not matter — exclusion
always wins.

```yaml
overrides:
  - glob: ["docs/security/*.md", "!docs/security/proto.md"]
    rules:
      max-file-length:
        max: 1000
```

A list containing only exclusion patterns matches nothing.

## Config globs (`ignore:`, `overrides:`, `kind-assignment:`)

Use the canonical `glob:` key for file patterns in
`overrides:` and `kind-assignment:` entries:

```yaml
overrides:
  - glob: ["docs/**/*.md", "!docs/research/**"]
    rules:
      line-length:
        max: 120

kind-assignment:
  - glob: ["plan/[0-9]*_*.md"]
    kinds: [plan]
```

### Migrating from the `files:` key

The `files:` key was the original field name for these
patterns. It continues to work but emits a deprecation
warning at load time:

```text
overrides[0]: `files:` is deprecated; rename it to `glob:` — see docs/reference/globs.md
```

Replace the `files:` key inside each `overrides:` or
`kind-assignment:` entry with `glob:` in your
`.mdsmith.yml`. The top-level `files:` discovery key is
unrelated and is not deprecated. The deprecated `files:`
key will be removed in a future release.

Note: the old `files:` key used a matcher where `*`
crossed path separators. The canonical `glob:` key uses
`doublestar` semantics where `*` matches a single path
component. Update patterns like `docs/guides/*.md` to
`docs/guides/**/*.md` if they relied on that behavior.

## Directive globs (`<?catalog?>`)

`<?catalog?>` accepts a `glob:` parameter whose patterns
are split on newlines into a list. A YAML `glob:` list
becomes one pattern per line.

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
are expanded with `doublestar.FilepathGlob`. It supports
the same `**` and brace-expansion syntax as config
patterns. `!`-prefix exclusion is not available on the
CLI; use `ignore:` in `.mdsmith.yml` instead.

```bash
mdsmith check 'docs/**/*.md'   # works, recurses into subdirs
mdsmith check '*.{md,markdown}'  # works, brace expansion
```
