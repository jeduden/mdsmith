---
id: MDS043
name: no-reference-style
status: ready
description: Reference-style links and footnotes require global definition resolution; flag them in favor of inline links.
---
# MDS043: no-reference-style

Reference-style links and footnotes require global definition
resolution; flag them in favor of inline links.

Inline links (`[text](url)`) carry their target locally. Reference
links (`[text][id]` plus `[id]: url`) and footnotes (`[^slug]` plus
`[^slug]: ...`) require a second pass over the document to resolve.
Forbidding them keeps every link diff readable in isolation.

## Settings

| Setting           | Type | Default | Description                                                                                                  |
|-------------------|------|---------|--------------------------------------------------------------------------------------------------------------|
| `allow-footnotes` | bool | `false` | Opt back into footnotes. Numeric slugs and definitions placed away from the referencing paragraph still fail |

## Config

Enable:

```yaml
rules:
  no-reference-style: true
```

Allow footnotes when the slug is meaningful and the definition sits
right after its referencing paragraph:

```yaml
rules:
  no-reference-style:
    allow-footnotes: true
```

Disable:

```yaml
rules:
  no-reference-style: false
```

## Examples

### Bad -- full reference link

<?include
file: bad/full-reference.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Full

See [example][site] for more.

[site]: https://example.com
```

<?/include?>

### Bad -- shortcut reference link

<?include
file: bad/shortcut-reference.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Shortcut

See [example] for more.

[example]: https://example.com
```

<?/include?>

### Bad -- footnote (default)

<?include
file: bad/footnote-disabled.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Footnote

A claim here.[^1]

[^1]: A note.
```

<?/include?>

### Bad -- unused reference definition

<?include
file: bad/unused-definition.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Unused

Some plain prose with no links.

[orphan]: https://example.com
```

<?/include?>

### Good -- inline links

<?include
file: good/inline.md
wrap: markdown
?>

```markdown
# Inline Links

See [example](https://example.com) for more.

Also see [home](https://example.org) for the project page.
```

<?/include?>

### Good -- footnote when allowed

<?include
file: good/footnote-allowed.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Footnotes Allowed

A claim with a citation.[^source]
[^source]: Author, "Title", 2024.
```

<?/include?>

## Diagnostics

| Message                                           | Meaning                                                                 |
|---------------------------------------------------|-------------------------------------------------------------------------|
| `reference-style link; use inline form ...`       | Full, collapsed, or shortcut reference link found                       |
| `footnote reference; footnotes are not allowed`   | A `[^slug]` reference appeared while `allow-footnotes` is false         |
| `footnote slug is numeric; use a meaningful slug` | `allow-footnotes` is true but the slug is purely digits                 |
| `footnote definition must follow ...`             | `allow-footnotes` is true but the definition is not adjacent to the ref |
| `unused reference definition: [id]`               | A reference definition has no matching link in the file                 |

## Meta-Information

- **ID**: MDS043
- **Name**: `no-reference-style`
- **Status**: ready
- **Default**: disabled, opt-in
- **Fixable**: yes (reference links only; footnotes are not auto-fixed)
- **Implementation**:
  [source](./)
- **Category**: link
