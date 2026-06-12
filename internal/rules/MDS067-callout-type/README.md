---
id: MDS067
name: callout-type
status: ready
description: Validate Obsidian callout types against an allowed set.
category: structural
nature: structure
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS067: callout-type

Validate Obsidian callout types against an allowed set.

A blockquote whose first paragraph begins with
`[!type]` is an Obsidian callout. MDS067 flags the
callout when `type` is not in the effective allow
set. Matching is case-insensitive. Custom types are
opt-in via the `allow` setting; the rule turns off
entirely under `allow-unknown: true`.

Disabled by default. Set `convention: obsidian` (or
toggle the rule on directly) to enable it.

## Settings

| Setting         | Type | Default | Description                                                    |
| --------------- | ---- | ------- | -------------------------------------------------------------- |
| `allow`         | list | `[]`    | Extra callout-type names to permit alongside the built-in set. |
| `allow-unknown` | bool | `false` | When `true`, accept any `[!name]` token without validation.    |

The built-in vocabulary is the Obsidian base set:

- `note`
- `abstract` (aliases `summary`, `tldr`)
- `info` (alias `todo`)
- `tip` (aliases `hint`, `important`)
- `success` (aliases `check`, `done`)
- `question` (aliases `help`, `faq`)
- `warning` (aliases `caution`, `attention`)
- `failure` (aliases `fail`, `missing`)
- `danger` (alias `error`)
- `bug`
- `example`
- `quote` (alias `cite`)

`allow` uses `append` merge semantics. A kind or
override that adds a custom type preserves entries
from earlier layers.

## Config

Enable:

```yaml
rules:
  callout-type: true
```

Disable:

```yaml
rules:
  callout-type: false
```

Extend the allowed set:

```yaml
rules:
  callout-type:
    allow:
      - review
      - decision
```

Accept any type:

```yaml
rules:
  callout-type:
    allow-unknown: true
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Standard Callouts

> [!note]
> A note callout.

Some prose between callouts.

> [!warning]
> A warning callout.

More prose between callouts.

> [!tip]
> A tip callout.
```

<?/include?>

### Bad

<?include
file: bad/unknown.md
wrap: markdown
?>

```markdown
# Bad Callout

> [!REVIEW]
> Unknown type.
```

<?/include?>

## Meta-Information

- **ID**: MDS067
- **Name**: `callout-type`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**: [source](./)
- **Category**: structural
