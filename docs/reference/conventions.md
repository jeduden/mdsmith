---
weight: 20
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
as a base layer beneath your own rule config. It
answers "what kind of Markdown does this project
write?" with one config knob instead of eight.

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
`overrides:`. An unknown name is a config error.

Built-in values: `portable`, `github`, `obsidian`,
`parity`, `plain`, `no-llm-tells`. The key is
optional; omit it for no convention.

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
| ------------------------ | ------------------------------------------------------- |
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
| ------------------- | -------------------------------------- |
| `markdown-flavor`   | `flavor: gfm`                          |
| `no-inline-html`    | `allow: [details, summary]`            |
| `emphasis-style`    | `bold: asterisk`, `italic: underscore` |
| `list-marker-style` | `style: dash`                          |

### `obsidian`

Markdown written in an Obsidian vault. Selects
`flavor: gfm` and turns on the Obsidian-specific
validations — MDS027 resolves `[[Page]]` wikilink
targets workspace-wide, and MDS067 checks every
`[!type]` callout against the Obsidian type set.

| Rule                             | Setting                                       |
| -------------------------------- | --------------------------------------------- |
| `markdown-flavor`                | `flavor: gfm`                                 |
| `cross-file-reference-integrity` | `wikilinks: true`, `wikilink-style: obsidian` |
| `callout-type`                   | enabled (12 base Obsidian types and aliases)  |

Standard style rules stay at their defaults so an
Obsidian vault behaves like a GFM project unless the
team layers more rules on top.

To run these checks inside the editor, install the
[Obsidian plugin](../guides/editors/obsidian.md). It
hosts the same engine as a WebAssembly runtime.

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

### `parity`

Markdown checked the way the
markdownlint-compatible tools (mado, rumdl) check
it: `flavor: gfm` plus every mdsmith-only rule
turned off, leaving the structural style class
those tools also run. Use it for a like-for-like
comparison, or as a fast markdownlint gate before
you adopt the cross-file and generated-section
layer. The disabled-rule list is generated from the
convention, so it never drifts — see the
[benchmark doc][parity-list]. Unlike the other
built-ins, `parity` leaves `markdown-flavor`
(MDS034) opt-in.

[parity-list]: ../research/benchmarks/README.md

### `no-llm-tells`

Flags mechanical LLM-prose tells in CI. MDS056
blocks vocabulary and phrasal tells; MDS055 blocks
banned sentence openers; MDS023 and MDS024 tighten
readability budgets. Lists are sourced from
[`slop-patterns.md`][slop]; a drift-checker test
keeps the two in sync.

Pins no flavor and does not enable `markdown-flavor`
(MDS034). The `contains` and `starts` lists merge by
**append**: a project's own entries join the
convention's list instead of replacing it.

[slop]: ../../.claude/skills/docs-author/slop-patterns.md

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
`Enabled: false` would override the convention's
`Enabled: true`.

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

A bool-only `markdown-flavor: false` entry toggles
`enabled` without erasing the preset's settings. The
rule stays configured but its `Check()` is gated
off. The other rules in the preset are untouched.

## User-defined conventions

The built-in conventions cover common cases. Teams
that need something custom define it inline in
`.mdsmith.yml`. The top-level `conventions:` key holds
the map:

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

Each entry is a `{ flavor, rules }` pair. The `rules`
block uses the same schema as the top-level `rules:`
block. To lift one out of `.mdsmith.yml` into its own
file under `.mdsmith/conventions/<name>.yaml`, see the
[convention files reference](convention-files.md).

### Validation

User-defined conventions are validated at config load:

- `flavor` must be a recognised flavor string such as
  `commonmark`, `gfm`, or `goldmark`.
- Each key under `rules:` must name a registered rule.
- Each rule's settings must pass the rule's own schema
  check.

Validation errors name the convention and the rule:

```text
convention "our-team" rule "no-inline-html": no-inline-html: unknown setting "allowed"
```

### Reserved names

The built-in names `portable`, `github`,
`obsidian`, `parity`, `plain`, and `no-llm-tells`
are reserved. Defining a `conventions.portable`
entry is a config error. This keeps the built-in
names stable across docs and tutorials.

### Resolution order

The lookup checks user-defined conventions first,
then falls back to the built-in table. Collisions with
reserved names are rejected at load time, so shadowing
is impossible. When neither table matches, the error
lists both sets:

```text
unknown convention "bogus" (valid: github, no-llm-tells, obsidian, our-team, parity, plain, portable)
```

### Interaction with top-level rules

User-defined conventions apply as a base layer, like
the built-in conventions. A top-level `rules:` entry
overrides the convention preset for that rule; the
rest of the preset remains. `mdsmith kinds resolve
<file>` labels user-convention layers with a
`(user)` suffix.

## Inspecting an effective convention

`mdsmith kinds resolve <file>` shows the merge
chain for every rule, including the
`convention.<name>` layer. Use it to confirm which
value won and where it came from.
