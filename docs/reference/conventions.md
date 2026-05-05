---
summary: >-
  Built-in Markdown conventions, the rule presets
  each one applies, and how user config layers on
  top via deep-merge.
---
# Markdown conventions

A **convention** is a named bundle that pairs a
Markdown flavor with rule presets. Setting
`convention:` in `.mdsmith.yml` selects one bundle.
The presets apply as a base layer beneath your own
rule config.

Conventions answer "what kind of Markdown does this
project write?" with one config knob instead of
eight.

A convention is distinct from a flavor. Flavor is a
property of the *renderer* (CommonMark, GFM,
goldmark — what the parser interprets). Convention
is a property of the *project* (the team's writing
choices among forms the renderer treats equally).
See the
[concepts doc](../background/concepts/flavor-rule-convention-kind.md)
for the full picture and where the concepts overlap.

## Selecting a convention

```yaml
convention: portable
```

That single line pins a flavor and a curated set of
style-rule settings. `convention:` is a top-level
config key, sibling to `rules:`, `kinds:`, and
`overrides:`. Setting an unknown name is a config
error at load time.

Built-in values: `portable`, `github`, `plain`. You
can also define your own — see
[User-defined conventions](#user-defined-conventions)
below. The key is optional; omit it for no
convention.

You may also set `flavor:` inside `markdown-flavor`
alongside `convention:`. If both are set, they must
agree — a convention that requires `commonmark`
rejects `flavor: gfm` at config load.

## Built-in conventions

### `portable`

Markdown that renders the same in every CommonMark
parser. Selects `flavor: commonmark` and turns on
the strict-style rules with their recommended
defaults.

| Rule                     | Setting                                                 |
|--------------------------|---------------------------------------------------------|
| `markdown-flavor`        | `flavor: commonmark`                                    |
| `no-inline-html`         | enabled                                                 |
| `no-reference-style`     | `allow-footnotes: false`                                |
| `emphasis-style`         | `bold: asterisk`, `italic: underscore`                  |
| `horizontal-rule-style`  | `style: dash`, `length: 3`, `require-blank-lines: true` |
| `list-marker-style`      | `style: dash`                                           |
| `ordered-list-numbering` | `style: sequential`, `start: 1`                         |
| `ambiguous-emphasis`     | `max-run: 2`                                            |

### `github`

Markdown that renders well on github.com. Selects
`flavor: gfm` and keeps the style rules light: the
inline-HTML allowlist permits `<details>` and
`<summary>`; emphasis and list-marker style are
pinned for consistency; the rest of the strict
rules stay off.

| Rule                | Setting                                |
|---------------------|----------------------------------------|
| `markdown-flavor`   | `flavor: gfm`                          |
| `no-inline-html`    | `allow: [details, summary]`            |
| `emphasis-style`    | `bold: asterisk`, `italic: underscore` |
| `list-marker-style` | `style: dash`                          |

### `plain`

Markdown that survives `cat`. The rendered output
should look about the same as the source viewed in
a plaintext reader. Same activations as `portable`,
plus `allow-comments: false` on `no-inline-html` so
HTML comments do not leak through as literal
`<!-- ... -->` text.

A truly plaintext-faithful convention needs three
more rules. One forbids `*` and `_` runs. One
requires indented code blocks. One inverts
`no-bare-urls` so bare URLs are preferred over
Markdown links. Those rules don't exist yet. When
they ship, the `plain` convention gains them and
diverges from `portable`.

## How presets layer with user config

Convention presets sit between built-in defaults
and your explicit top-level rules. The merge order,
oldest → newest, is:

1. `default` — built-in defaults: rules in
   `cfg.Rules` that you did not set
2. `convention.<name>` — the preset table
3. `user` — your top-level rules block (rules you
   explicitly set in `.mdsmith.yml`)
4. `kinds.<name>` — each kind in the file's
   effective list
5. `overrides[i]` — each matching override entry

Each layer deep-merges onto the previous one.
Scalars at a leaf are replaced by the later layer;
maps recurse key by key; lists replace by default.
A convention preset provides the floor; your
explicit `rules:` block overrides on top.

The `default` and `user` layers come from the same
`cfg.Rules` map. mdsmith splits them around the
convention so a convention can enable a rule that
is opt-in by default (e.g. `convention: portable`
turns on MDS034). Without the split, the default's
`Enabled: false` would land on top of the
convention's `Enabled: true` and silently disable
the rule.

For example, the `github` convention sets
`no-inline-html.allow: [details, summary]`. To
extend the allowlist with `<sub>` and `<sup>`,
write:

```yaml
convention: github
rules:
  no-inline-html:
    allow: [sub, sup]
```

Lists default to replace, so the effective
allowlist becomes `[sub, sup]`. To keep the
preset's entries, list them explicitly:
`allow: [details, summary, sub, sup]`.

## User-defined conventions

If none of the built-in conventions fits your
project, define your own inline in `.mdsmith.yml`:

```yaml
conventions:
  our-team:
    flavor: gfm
    rules:
      no-inline-html:
        allow: [details, summary, kbd]
      list-marker-style:
        style: dash
      no-reference-style:
        allow-footnotes: true

convention: our-team
```

The `conventions:` block is a top-level map, sibling
to `kinds:` and `overrides:`. Each entry has the
same shape as a built-in convention: an optional
`flavor:` string and an optional `rules:` block.

The `flavor:` value must be one of `commonmark`,
`gfm`, or `goldmark`. The `rules:` block uses the
same schema as the top-level `rules:` block: a bool
to enable or disable, or a map of settings.

Validation errors are reported at config load time
and name both the convention and the rule:

```text
convention "our-team" rule "no-inline-html": unknown setting "allowed"
```

### Name collisions

The built-in names `portable`, `github`, and `plain`
are reserved. Defining a `conventions.portable`
entry in `.mdsmith.yml` is a config error. This
keeps documentation and tutorials consistent: when
you read "use `convention: portable`", it always
means the built-in table.

### Resolution order

When `convention: <name>` resolves:

1. Look up the name in the user-defined
   `conventions:` map first.
2. Fall back to the built-in table.
3. If neither matches, emit a config error listing
   both sets of names.

Because reserved names are rejected at parse time,
user conventions cannot shadow built-ins.

### Identifying user conventions in output

`mdsmith kinds resolve` shows the convention layer
with a `(user)` suffix when the active convention is
user-defined:

```text
convention.our-team (user)    set    true
```

Built-in conventions show no suffix:

```text
convention.portable           set    true
```

## Disabling MDS034

A convention applies its rule presets at config
load time. Disabling `markdown-flavor` itself does
not disable the rules a convention turned on.

```yaml
convention: portable
rules:
  markdown-flavor: false
```

The `convention:` selector lives at the top level.
So the user can disable MDS034 cleanly with a
bool-only `markdown-flavor: false` entry in the
rules block. The convention preset has already
populated the merged config at load time. A
bool-only later layer toggles `enabled` without
erasing the preset's settings. The rule stays
configured but its `Check()` is gated off. The
other rules in the preset are untouched.

This split keeps MDS034 focused on "what does this
renderer interpret as a feature." Conventions
orchestrate style separately.

## Inspecting an effective convention

`mdsmith kinds resolve <file>` shows the merge
chain for every rule, including the
`convention.<name>` layer. Use it to confirm which
value won and where it came from.
