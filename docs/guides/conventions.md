---
title: Use a Markdown convention
weight: 32
summary: >-
  Select a built-in convention, declare your own
  inline, layer rules over its preset, keep the
  flavor in agreement, and split a convention into
  its own file.
---
# Use a Markdown convention

A convention bundles a Markdown flavor with a set of
rule settings under one name. Select one to answer
"what kind of Markdown does this project write?" with
a single config key instead of eight separate rule
blocks. This guide covers picking a built-in,
declaring your own, and the rules that govern both.

For the full preset tables and the merge algorithm,
see the [conventions reference](../reference/conventions.md).

## Pick a built-in or define your own

Start with a built-in convention when one fits. The
shipped names are `portable`, `github`, `obsidian`,
`plain`, `no-llm-tells`, and four `<linter>-parity`
conventions. Each pins a
flavor and a curated rule preset; the
[conventions reference](../reference/conventions.md)
lists what every one turns on.

Define your own when no built-in matches your team's
choices — a different inline-HTML allowlist, a
project-specific list-marker style, a footnote policy.
A user convention has the same `{ flavor, rules }`
shape as a built-in and layers the same way.

The built-in names are reserved. A
`conventions.portable` entry is a config error, so
your names never shadow a built-in.

## Select a convention

Set the top-level `convention:` key in `.mdsmith.yml`:

```yaml
convention: portable
```

`convention:` is a top-level key, sibling to `rules:`,
`kinds:`, and `overrides:`. It names one bundle. An
unknown name is a config error that lists the valid
names. Omit the key for no convention.

## Declare a convention inline

Put a custom convention under the top-level
`conventions:` key. Each entry is a `{ flavor, rules }`
pair; the `rules:` block uses the same schema as the
top-level `rules:` block.

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

Declaring the bundle does not select it. Add the
`convention: our-team` line to apply it.

mdsmith validates a user convention at config load:
the `flavor:` must be a recognised flavor string, each
key under `rules:` must name a registered rule, and
each rule's settings must pass that rule's own schema
check. An error names the convention and the rule.

## Keep the flavor in agreement

A convention pins a flavor. Setting `flavor:` under
`markdown-flavor` at the same time is allowed only when
the two agree:

```yaml
convention: portable     # requires commonmark
rules:
  markdown-flavor:
    flavor: commonmark    # agrees — accepted
```

A conflicting flavor is a config error. `convention:
portable` with `flavor: gfm` is rejected at load,
because the convention promises CommonMark-portable
output and GFM would break that promise. Drop the
explicit `flavor:` and let the convention set it, or
pick a convention whose flavor you want.

## Layer rules over the preset

A convention preset is a base layer, not a lock. Your
top-level `rules:` block overrides it rule by rule —
keys you set replace the preset's, keys you omit keep
the preset's value.

```yaml
convention: github       # sets no-inline-html.allow: [details, summary]
rules:
  no-inline-html:
    allow: [details, summary, sub, sup]
```

List settings replace by default, so name every entry
you want to keep. The example above restates `details`
and `summary` to extend the allowlist rather than
shrink it to `[sub, sup]`.

To turn a preset rule off without losing its settings,
set it to `false`:

```yaml
convention: portable
rules:
  markdown-flavor: false
```

The bool-only entry gates the rule's check off but
leaves the preset's other rules untouched.

## Split a convention into its own file

When the `conventions:` block grows large, move one
convention into a file under `.mdsmith/conventions/`.
The basename is the convention name. Cut the body and
delete the inline entry:

```yaml
# .mdsmith/conventions/our-team.yaml
flavor: gfm
rules:
  no-inline-html:
    allow: [details, summary, kbd]
  list-marker-style:
    style: dash
```

Keep the `convention: our-team` selector in
`.mdsmith.yml` — a convention file supplies the bundle,
never picks it. Move the body, do not copy it:
declaring the same name both inline and in a file is a
config error naming both sources. The effective rules
are byte-equal either way. The
[convention files reference](../reference/convention-files.md)
covers the basename rule and the directory layout.

## Confirm what won

Run `mdsmith kinds resolve <file>` to see the merge
chain for every rule, including the `convention.<name>`
layer. A user-convention layer carries a `(user)`
suffix, so you can tell a custom bundle from a
built-in and see which value won at each rule.

## See also

- [Conventions reference](../reference/conventions.md)
  — the built-in preset tables and the merge order.
- [Convention files](../reference/convention-files.md)
  — one file per user convention.
- [Flavor, rule, convention, kind](../background/concepts/flavor-rule-convention-kind.md)
  — how the four concepts differ and compose.
