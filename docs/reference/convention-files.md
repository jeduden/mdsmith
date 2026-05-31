---
title: Convention files under `.mdsmith/conventions/`
weight: 22
summary: >-
  Each file under `.mdsmith/conventions/` declares one
  user convention. The basename is the convention name;
  the file body carries a `flavor:` plus a `rules:` map.
  Sits alongside inline `conventions.<name>:` in
  `.mdsmith.yml`.
---
# Convention files under `.mdsmith/conventions/`

A **convention file** is a YAML file under
`.mdsmith/conventions/` whose basename is the
convention's name and whose body is the full convention
bundle. One file per convention, no nesting. The
directory sits next to `.mdsmith.yml` at the workspace
root.

```text
.mdsmith.yml                   # unchanged
.mdsmith/
  kinds/                       # plan 208
    audit-log.yaml
  conventions/
    portable-strict.yaml
    long-form-docs.yaml
```

Use convention files when the `conventions:` block has
grown large. Each rule edit dirties the same
`.mdsmith.yml` as every other config change. Splitting
conventions into one file each isolates the history. The
read path shortens too: open `portable-strict.yaml` to
see the whole `portable-strict` convention.

Built-in conventions (`portable`, `github`, `plain`, and
the rest listed in the
[conventions reference](conventions.md)) stay compiled
into the binary. Convention files hold only the
conventions you define.

## File shape

The file body matches the inline `conventions.<name>:`
body — a [`UserConvention`](conventions.md): a `flavor:`
key plus a `rules:` map. The `rules:` block uses the same
schema as the top-level `rules:` block. A key outside
that set is a config error naming the key and file.

```yaml
# .mdsmith/conventions/portable-strict.yaml
flavor: commonmark
rules:
  line-length:
    max: 72
  no-bare-urls: true
  no-inline-html:
    allow: [details, summary]
```

A convention must declare a `flavor:` to be selectable.
The flavor must be a recognised flavor string such as
`commonmark`, `gfm`, or `goldmark`. Each key under
`rules:` must name a registered rule and pass that rule's
own schema check, exactly as an inline convention does.

Select the convention the same way as any other — with
the top-level `convention:` key in `.mdsmith.yml`:

```yaml
convention: portable-strict
```

The `convention:` selector stays in `.mdsmith.yml`; it is
not externalized. A convention file only supplies the
bundle, never picks it.

## Basename rule

The convention's name is the basename minus extension.
The basename must match `[a-z][a-z0-9-]*` — lower case,
starting with a letter, with optional hyphen-separated
segments. The rule applies only to filenames (OS case
folding, path safety); inline `conventions.<name>:` keys
stay unvalidated.

Both `*.yaml` and `*.yml` are scanned. Two convention
files with the same basename across the two extensions is
a config error naming both files.

Subdirectories under `.mdsmith/conventions/` are
rejected, as are symlinks. A file larger than 1 MB is
rejected. One convention per file, flat layout.

## Composition with `.mdsmith.yml`

`conventions.<name>:` blocks inside `.mdsmith.yml` remain
a first-class source. A project can mix inline and
file-defined conventions freely.

The same convention name declared in **both** a file and
inline is a config error naming both sources. The two
sources do **not** merge — a merged convention would
defeat the "read one file to know one convention"
property convention files ship.

A name colliding with a built-in convention (`portable`,
`github`, `plain`, and the others in the
[conventions reference](conventions.md)) is a config
error: the built-in name is reserved. This keeps the
built-in names stable across docs and tutorials.

The top-level `convention:` selector and the
`overrides:`, `kinds:`, and `ignore:` blocks all stay in
`.mdsmith.yml`. A `convention: <name>` entry references a
convention by name — inline or file convention — with no
extra wiring.

## Splitting an inline convention

To move an existing inline convention into its own file,
cut the body under `conventions.<name>:` and drop the
inline entry. Take this `.mdsmith.yml`:

```yaml
conventions:
  our-team:
    flavor: gfm
    rules:
      no-inline-html:
        allow: [details, summary, kbd]
      list-marker-style:
        style: dash

convention: our-team
```

Write the body to a file named for the convention, and
delete the `conventions:` block:

```yaml
# .mdsmith/conventions/our-team.yaml
flavor: gfm
rules:
  no-inline-html:
    allow: [details, summary, kbd]
  list-marker-style:
    style: dash
```

The `convention: our-team` selector stays in
`.mdsmith.yml`. Move the body, do not copy it: declaring
the same name both inline and in a file is a config error
naming both sources. The effective rules are byte-equal
either way.

## Audit

`mdsmith kinds resolve <file>` prints the active
convention and the file that defined it, so you can jump
straight to the right source:

```text
file: docs/guide.md
effective kinds:
  (none)
convention: portable-strict (user) defined-in .mdsmith/conventions/portable-strict.yaml
```

The `(user)` tag marks a user-defined convention.
Built-in conventions carry no tag and no defining-source
path — they are compiled into the binary.

The JSON shape (`--json`) carries a `convention` object
with `name` and `source-path` keys. That parallels each
resolved kind's `source-path`, so editor integrations can
key off a stable field.

`mdsmith kinds resolve <file>` also shows the full merge
chain for every rule, including the `convention.<name>`
layer. Use it to confirm which value won and where it
came from.

See the [conventions reference](conventions.md) for the
built-in bundles, the merge order, and how presets layer
with your top-level rules.
