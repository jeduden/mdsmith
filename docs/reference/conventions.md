---
summary: >-
  Built-in Markdown conventions, the rule presets
  each one applies, and how user config layers on
  top via deep-merge.
---
# Markdown conventions

A **convention** is an opinionated bundle of rule
settings that pairs a Markdown flavor with a set of
style choices. Setting `convention:` at the top of
your `.mdsmith.yml` selects one of the built-in
bundles; the rule presets in that bundle are applied
as a base layer beneath your own rule config.

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

Built-in values: `portable`, `github`, `plain`. The
key is optional; omit it for no convention.

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

## User-defined conventions

A team that needs a convention that does not match
any built-in can define one inline in
`.mdsmith.yml`:

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

Each entry under `conventions:` has the same `{
flavor, rules }` shape as the built-in table.
`flavor` must be one of `commonmark`, `gfm`, or
`goldmark`. The `rules` block uses the same schema
as the top-level `rules:` block.

### Reserved names

The names `portable`, `github`, and `plain` are
reserved. Defining a `conventions.portable` (or
`github` / `plain`) in `.mdsmith.yml` is a config
error. The reserved names keep docs and tutorials
consistent.

### Resolution order

When the `convention:` key is resolved:

1. Look up the name in user-defined `conventions:`
   first.
2. Fall back to the built-in table.
3. If neither matches, emit a config error listing
   all known names from both sets.

User conventions cannot shadow built-ins. Name
collisions are rejected at parse time.

### Validation

User-defined conventions are validated at config
load:

- `flavor` must be one of `commonmark | gfm |
  goldmark`.
- Each key under `rules:` that names a registered
  rule has its settings validated via the rule's
  existing schema. Unknown rule names are silently
  accepted for forward-compatibility (presets for
  upcoming rules can ship early).

Validation errors name the convention and the rule:

```text
convention "our-team" rule "list-marker-style":
unknown style "bogus"
```

### Interaction with top-level rules

User-defined conventions layer exactly like
built-in conventions — they are a base layer
beneath any top-level `rules:` overrides. A team
can set `convention: our-team` and then override
one rule in the top-level `rules:` block; the
override wins.

### Inspecting user conventions

`mdsmith kinds resolve <file>` labels user
convention layers with a `(user)` suffix so they
are easy to distinguish from built-ins:

```text
convention.our-team (user)  set  true
```

## Inspecting an effective convention

`mdsmith kinds resolve <file>` shows the merge
chain for every rule, including the
`convention.<name>` layer. Use it to confirm which
value won and where it came from.
