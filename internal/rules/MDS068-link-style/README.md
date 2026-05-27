---
id: MDS068
name: link-style
status: ready
description: Flag links whose path, extension, or inline-vs-reference form deviates from the project's declared `links.style` policy.
category: link
nature: style
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
---
# MDS068: link-style

Flag links whose path, extension, or inline-vs-reference form
deviates from the project's declared `links.style` policy.

This rule closes gap G8 in the
[link handling audit](../../../docs/research/links/README.md).
It reads the shared `links:` config block — the same block MDS027
reads for its own settings. Adding `link-style` keeps the block
as one source of truth per kind: `site-root`, `validate-images`,
and `validate-reference-style` flow to MDS027, while `style` and
`external-skip` flow to this rule.

## Settings

| Setting                 | Type   | Default | Description                                                                                                                    |
| ----------------------- | ------ | ------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `links.style.path`      | string | `""`    | `relative` flags absolute targets; `absolute` flags relative targets                                                           |
| `links.style.extension` | string | `""`    | `keep` flags Markdown-shaped targets without a markdown extension; `strip` flags those with `.md`/`.markdown`                  |
| `links.style.form`      | string | `""`    | `inline` flags reference-style links; `reference` flags inline links; `any` is permissive                                      |
| `links.external-skip`   | list   | `[]`    | Regex patterns reserved for the future external-link-check rule (issue #47); parsed here so users can declare it once per kind |

Each style axis is independent. An empty string disables that
axis without affecting the other two. Enabling the rule with all
three axes unset is a no-op — useful when the rule is on at the
project level but a kind opts out by clearing one axis.

External URLs (`http:`, `https:`, `mailto:`), local-anchor-only
references, and images are not checked. The `path` and `form`
axes apply to every local text link, including non-Markdown
targets like `theme.css` — consistency is the point. The
`extension` axis is the only Markdown-shaped axis, described
below.

The extension policy only applies to Markdown-shaped targets — a
target whose last segment ends in `.md` or `.markdown`, or has no
extension. Targets ending in any other extension (`.png`, `.css`,
…) are ignored by the `extension` axis regardless of
`keep`/`strip`. Both `.md` and `.markdown` are treated as the
same "with extension" form.

Directory-style targets (trailing `/`, `.`, `..` — e.g.
`/docs/rules/MDS027/` or `../`) reference a rendered page
directory rather than a file. The `extension` axis also skips
these, since there is no filename segment to judge.

A note on adopting `extension: strip`. MDS027
[cross-file-reference-integrity](../MDS027-cross-file-reference-integrity/README.md)
checks `.md` and `.markdown` targets by default. It
silently skips extensionless ones unless `strict:
true` is also set. So an extensionless target with a
broken link will pass MDS027 silently. To keep that
check on, also set `cross-file-reference-integrity.strict: true`.

## Config

Enable with the default permissive policy:

```yaml
rules:
  link-style: true
```

Pin a project-wide style:

```yaml
rules:
  link-style:
    links:
      style:
        path: relative
        extension: keep
        form: inline
```

Override per kind. Rule READMEs are pure relative inline; the
`docs` kind clears form back to any until the corpus is migrated:

```yaml
kinds:
  rule-readme:
    rules:
      link-style:
        links:
          style:
            path: relative
            extension: keep
            form: inline
  docs:
    rules:
      link-style:
        links:
          style:
            form: any
```

Disable:

```yaml
rules:
  link-style: false
```

## Examples

### Bad -- absolute target under style.path=relative

<?include
file: bad/path-relative.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Path Relative

See [docs](/docs/target.md).
```

<?/include?>

### Bad -- .md suffix under style.extension=strip

<?include
file: bad/extension-strip.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Extension Strip

See [target](bad/sub/target.md).
```

<?/include?>

### Bad -- reference link under style.form=inline

<?include
file: bad/form-inline.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Form Inline

See [target][label].

[label]: sub/target.md
```

<?/include?>

### Good -- relative path

<?include
file: good/path-relative.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Path Relative

See [sibling](good/sibling.md) for a relative link target.
```

<?/include?>

### Good -- extensionless link

<?include
file: good/extension-strip.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Extension Strip

See [target](good/sibling) — no .md suffix, as policy requires.
```

<?/include?>

### Good -- inline link

<?include
file: good/form-inline.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Form Inline

See [sibling](good/sibling.md) — inline form, as policy requires.
```

<?/include?>

## Diagnostics

| Condition                                              | Message                                                                                 |
| ------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| absolute target under `style.path=relative`            | `link target is absolute; style.path=relative requires a relative path`                 |
| relative target under `style.path=absolute`            | `link target is relative; style.path=absolute requires an absolute path`                |
| extensionless under `style.extension=keep`             | `link target has no markdown extension; style.extension=keep requires .md or .markdown` |
| `.md`/`.markdown` suffix under `style.extension=strip` | `link target has a markdown extension; style.extension=strip forbids .md and .markdown` |
| reference-style under `style.form=inline`              | `reference-style link; style.form=inline requires inline form [text](url)`              |
| inline under `style.form=reference`                    | `inline link; style.form=reference requires reference form [text][label]`               |

## See also

- [Link handling audit](../../../docs/research/links/README.md) —
  the G8 gap this rule closes.
- [MDS027 cross-file-reference-integrity](../MDS027-cross-file-reference-integrity/README.md)
  — shares the `links:` config block.
- [MDS043 no-reference-style](../MDS043-no-reference-style/README.md)
  — a hard ban on reference style. MDS068 with `form: inline` is
  the softer per-kind equivalent.

## Meta-Information

- **ID**: MDS068
- **Name**: `link-style`
- **Status**: ready
- **Default**: disabled, opt-in. When enabled, every style axis
  defaults to `""` (no check) until set explicitly.
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: link
