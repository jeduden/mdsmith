---
id: MDS068
name: link-style
status: ready
description: Flag links and images whose path, extension, form, or link-image-style deviates from the project's declared `links.style` policy.
category: link
nature: style
maintainability: null
markdownlint:
  - id: MD054
    name: link-image-style
    partial: false
    default: true
rumdl:
  - id: MD054
    name: link-image-style
    partial: false
    default: true
mado: []
panache: []
obsidian-linter: []
---
# MDS068: link-style

Flag links and images whose path, extension, form, or link-image-style
deviates from the project's declared `links.style` policy.

This rule closes gap G8 in the
[link handling audit](../../../docs/research/links/README.md)
and implements markdownlint MD054 (link-image-style) in full.
It reads the shared `links:` config block — the same block MDS027
reads for its own settings. Adding `link-style` keeps the block
as one source of truth per kind: `site-root`, `validate-images`,
and `validate-reference-style` flow to MDS027, while `style` and
`external-skip` flow to this rule.

## Settings

| Setting                                     | Type   | Default | Description                                                                                                                    |
| ------------------------------------------- | ------ | ------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `links.style.path`                          | string | `""`    | `relative` flags absolute targets; `absolute` flags relative targets                                                           |
| `links.style.extension`                     | string | `""`    | `keep` flags Markdown-shaped targets without a markdown extension; `strip` flags those with `.md`/`.markdown`                  |
| `links.style.form`                          | string | `""`    | `inline` flags reference-style links; `reference` flags inline links; `any` is permissive                                      |
| `links.style.link-image-style`              | map    | absent  | MD054 per-style toggles; absent means the axis is inactive (see below)                                                         |
| `links.style.link-image-style.autolink`     | bool   | `true`  | Allow or forbid `<https://x>` autolinks                                                                                        |
| `links.style.link-image-style.inline`       | bool   | `true`  | Allow or forbid `[t](u)` inline links                                                                                          |
| `links.style.link-image-style.full`         | bool   | `true`  | Allow or forbid `[t][label]` / `![alt][label]` full reference links and images                                                 |
| `links.style.link-image-style.collapsed`    | bool   | `true`  | Allow or forbid `[t][]` / `![alt][]` collapsed reference links and images                                                      |
| `links.style.link-image-style.shortcut`     | bool   | `true`  | Allow or forbid `[t]` / `![alt]` shortcut reference links and images                                                           |
| `links.style.link-image-style.inline-image` | bool   | `true`  | Allow or forbid `![alt](src)` inline images                                                                                    |
| `links.external-skip`                       | list   | `[]`    | Regex patterns reserved for the future external-link-check rule (issue #47); parsed here so users can declare it once per kind |

Each style axis is independent. An empty string disables the string
axes without affecting the others. The `link-image-style` axis is
inactive unless at least one toggle is explicitly configured — an
enabled-but-unconfigured axis emits no diagnostics, matching
markdownlint's default behaviour for MD054. When `link-image-style`
is present but a toggle is omitted, the omitted toggle defaults to
`true` (allow).

The `form` axis (legacy three-value string) and the `link-image-style`
axis are independent. Both can be active simultaneously and each emits
its own diagnostics. Users who have already configured `form` are not
forced to migrate to `link-image-style`.

External URLs (`http:`, `https:`, `mailto:`), local-anchor-only
references are excluded from the `path`, `extension`, and `form`
axes. The `link-image-style` axis checks `<url>` autolinks, inline and
reference-style links, and inline and reference-style images. The
reference sub-form toggles (`full`, `collapsed`, `shortcut`) apply to
both links and images; `inline-image` governs only `![alt](src)`. The
`path` and `form` axes apply to
every local text link, including non-Markdown targets like `theme.css`.
The `extension` axis is the only Markdown-shaped axis, described below.

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

Enforce MD054 (inline and full reference only; forbid collapsed,
shortcut, and autolink):

```yaml
rules:
  link-style:
    links:
      style:
        link-image-style:
          autolink: false
          inline: true
          full: true
          collapsed: false
          shortcut: false
          inline-image: true
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

| Condition                                                      | Message                                                                                 |
| -------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| absolute target under `style.path=relative`                    | `link target is absolute; style.path=relative requires a relative path`                 |
| relative target under `style.path=absolute`                    | `link target is relative; style.path=absolute requires an absolute path`                |
| extensionless under `style.extension=keep`                     | `link target has no markdown extension; style.extension=keep requires .md or .markdown` |
| `.md`/`.markdown` suffix under `style.extension=strip`         | `link target has a markdown extension; style.extension=strip forbids .md and .markdown` |
| reference-style under `style.form=inline`                      | `reference-style link; style.form=inline requires inline form [text](url)`              |
| inline under `style.form=reference`                            | `inline link; style.form=reference requires reference form [text][label]`               |
| `<url>` autolink when `link-image-style.autolink=false`        | `autolink style forbidden; link-image-style.autolink=false`                             |
| `[t](u)` inline when `link-image-style.inline=false`           | `inline style forbidden; link-image-style.inline=false`                                 |
| `[t][label]` full ref when `link-image-style.full=false`       | `full reference style forbidden; link-image-style.full=false`                           |
| `[t][]` collapsed when `link-image-style.collapsed=false`      | `collapsed reference style forbidden; link-image-style.collapsed=false`                 |
| `[t]` shortcut when `link-image-style.shortcut=false`          | `shortcut reference style forbidden; link-image-style.shortcut=false`                   |
| `![alt](src)` image when `link-image-style.inline-image=false` | `inline-image style forbidden; link-image-style.inline-image=false`                     |

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
- **markdownlint**: [MD054][mdl-md054] (link-image-style)
- **rumdl**: [MD054][rumdl-md054] (link-image-style)

[mdl-md054]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md054.md
[rumdl-md054]: https://rumdl.dev/md054/
