---
id: 112
title: Markdown convention bundles for MDS034
status: "✅"
summary: >-
  Introduce a top-level `convention:` config key that
  pairs a Markdown flavor with a set of style-rule
  presets. Three conventions ship: portable
  (CommonMark everywhere), github (GFM on
  github.com), and plain (Markdown that survives
  `cat`). Convention applies as a base layer beneath
  the user's top-level rules.
model: opus
---
# Markdown convention bundles for MDS034

## Goal

Give a project one knob for "what kind of Markdown
do we write" without overloading the renderer's
flavor. Today MDS034 only answers which extensions a
renderer supports. Adding seven style rules
(MDS041-MDS047) without a bundle would make a
strict-Markdown setup take eight separate config
keys. A convention collapses those eight choices
into one.

## Background

### Three concepts, three scopes

mdsmith already names two related ideas. **Flavor**
is a property of a renderer — what CommonMark, GFM,
or goldmark recognize. **Kind** is a property of a
file — a role tag attached via front matter or
glob. Convention is the missing third concept: a
property of the *project* — what the team writes,
project-wide.

The three answer different questions:

- Flavor: "what *renderer* do we target?" (one)
- Convention: "what *kind of Markdown* does this
  project write?" (one)
- Kind: "what *role* does this file play?" (zero or
  many)

Conflating any two leads to broken designs. Treating
flavor as opinionated style would imply renderers
that don't exist. Treating convention as a kind
would suggest different files in the same project
target different renderers, which they cannot. See
the
[concepts doc](../docs/background/concepts/flavor-rule-convention-kind.md)
for the full distinction.

### Today's flavor model

The
[markdownflavor rule](../internal/rules/markdownflavor/)
defines several flavors. It emits diagnostics when
the configured flavor does not interpret a feature
the file uses.

```yaml
rules:
  markdown-flavor:
    flavor: gfm
```

The flavor controls only MDS034's own diagnostics.
Other rules (MDS012 no-bare-urls, MDS018
no-emphasis-as-heading, etc.) are configured
independently.

### What plans 105-111 add

Seven rules cover MDS041 through MDS047. Each
restricts a different CommonMark ambiguity. Without
a convention bundle they would each be enabled and
configured separately, multiplying config friction.

## Design

### Configuration

A new top-level `convention:` config key, sibling to
`rules:`, `kinds:`, and `overrides:`:

```yaml
convention: portable
```

That single line pins a flavor and a curated set of
style-rule settings. The convention applies as a
base layer beneath the user's `rules:` block. A
user can set `convention: portable` and adjust one
rule on top:

```yaml
convention: github
rules:
  no-inline-html:
    allow: [details, summary, sub, sup]
```

Convention is project-wide. There is one selection
per config file. Setting an unknown name is a config
error at load.

### Flavor and convention

A convention implies a flavor. Setting both is
allowed only when they agree. A mismatch is a
config error:

```yaml
convention: portable
rules:
  markdown-flavor:
    flavor: gfm     # ← error: portable requires commonmark
```

### Why top-level and not nested

An earlier draft put `profile:` inside the MDS034
rule's settings. That broke down for three reasons.
Disabling MDS034 (`rules.markdown-flavor: false`)
collided with `rules.markdown-flavor: { profile: ...
}` because YAML disallows duplicate mapping keys.
Discoverability was poor — a project-level concept
buried inside a per-rule settings block. Validation
ran in two places (the rule and the loader), forcing
the cross-check to live twice.

Top-level `convention:` fixes all three. It sits
where users look for project decisions. Disabling
MDS034 now works the obvious way. Validation is one
config-load pass.

### Why "convention" and not "profile"

"Profile" reads as a near-synonym of "flavor"
(both feel like "kind of Markdown"). The line
between them is real but the names blur it.
"Convention" reframes the concept around what it
actually is: the team's writing convention, distinct
from the renderer's grammar. Three concepts, three
words, no name collision: flavor (renderer),
convention (project), kind (file).

### Built-in conventions

Three conventions ship initially. Each names a
target.

- **`portable`** — Markdown that renders the same in
  every CommonMark parser. `flavor: commonmark`, all
  of MDS041-MDS047 enabled with recommended defaults
  (no inline HTML, no reference links, asterisk
  bold, underscore italic, dash HR, dash list
  marker, sequential ordered numbering, max emphasis
  run 2).
- **`github`** — Markdown that renders well on
  github.com. `flavor: gfm`, MDS041 enabled with
  `<details>` and `<summary>` in the allowlist,
  MDS042 and MDS045 enabled with defaults, the rest
  disabled.
- **`plain`** — Markdown that survives `cat`. Same
  activations as `portable` plus
  `allow-comments: false` on MDS041 (HTML comments
  leak through as literal text in plaintext readers).

The `plain` convention sits close to `portable`
today. A truly plaintext-faithful convention would
need rules that do not exist yet (no-emphasis,
no-fenced-code, prefer-bare-urls). When those rules
land in follow-up plans, `plain` gains them and
diverges from `portable`.

Conventions are declared in the
[markdownflavor package](../internal/rules/markdownflavor/conventions.go)
as a table: name → flavor + rule preset map.

### How preset application works

Conventions do not re-implement the rule logic.
They push settings into the existing rule config
during config load. The flow:

1. Config loader reads top-level `convention:`.
2. If non-empty, it looks up the convention table.
3. The preset is captured on `Config.ConventionPreset`.
4. `effectiveRules` merges the preset as the lowest
   layer (oldest), beneath `default`, kinds, and
   overrides.

This piggybacks on the existing deep-merge config
([plan 97](97_deep-merge-config.md)). User
overrides still win.

A team can set `convention: github` and extend the
inline-HTML allowlist:

```yaml
convention: github
rules:
  no-inline-html:
    allow: [details, summary, sub, sup]
```

Lists default to replace, so the effective allowlist
becomes `[sub, sup, details, summary]` only if the
user keeps the preset entries explicitly. The
preset provides the floor.

### MDS034 runtime changes

MDS034's `Check()` does not gain new diagnostics.
Its job stays the same: detect features the
configured flavor does not interpret. The convention
mechanism is a config-layer concern. The reasons:

- Each rule already owns its detection logic.
  Duplicating into MDS034 would create two places
  that emit the same diagnostic.
- Tests for MDS041-MDS047 should not depend on
  MDS034 being enabled.
- Disabling MDS034 should not silently disable the
  style rules a convention turned on.

MDS034 itself does **not** read the convention name.
It still only reads `flavor:` from its own settings.
The convention preset writes `flavor:` into MDS034's
settings as part of its preset, and MDS034's
existing ApplySettings handles it normally.

### Failure modes

- Unknown convention name → config error at load
  time with the list of valid names.
- Convention/flavor disagreement → config error
  naming both values.
- Convention is non-string → config error naming
  the field.

## Tasks

1. [x] Add a Convention type and built-in table in
   the
   [markdownflavor package](../internal/rules/markdownflavor/conventions.go)
   with a `Lookup(name string) (Convention, error)`
   helper.
2. [x] Add a top-level `Convention string` field on
   `config.Config` with a `convention:` YAML tag.
3. [x] Add `applyConvention(cfg *Config) error` in
   [internal/config/convention.go](../internal/config/convention.go)
   that runs after `ValidateKinds` in `Load`,
   resolves the convention name, validates against
   any user-set flavor, and stores the preset on
   `Config.ConventionPreset`.
4. [x] Update `effectiveRules` to merge
   `cfg.ConventionPreset` as the lowest layer
   beneath `cfg.Rules`.
5. [x] Add a `convention.<name>` layer to the kinds
   resolution output
   ([plan 95](95_kind-rule-resolution-cli.md)) so
   `mdsmith kinds resolve` shows which rules a
   convention turned on.
6. [x] Document the three built-in conventions in
   [docs/reference/conventions.md](../docs/reference/conventions.md).
7. [x] Cover convention vs flavor vs kind in the
   [concepts doc](../docs/background/concepts/flavor-rule-convention-kind.md).

## Acceptance Criteria

- [x] Setting `convention: portable` enables
      MDS041-MDS047 with documented preset values.
      The preset table ships the full set;
      activations against the live rule registry
      are deferred until plans 105-111 land those
      rules.
- [x] Setting `convention: github` enables MDS041,
      MDS042, MDS045 with their documented presets
      and leaves the rest disabled.
- [x] Setting `convention: plain` enables
      MDS041-MDS047 with the portable presets plus
      `allow-comments: false` on MDS041.
- [x] User overrides win over convention presets
      via deep-merge (e.g. extending the inline-HTML
      allowlist).
- [x] Setting both `convention: portable` and
      `rules.markdown-flavor.flavor: gfm` produces a
      config error naming both values.
- [x] Setting an unknown convention name produces a
      config error naming the field and listing the
      valid names.
- [x] Setting `convention:` to a non-string value
      produces a config error.
- [x] `mdsmith kinds resolve` reports which rules a
      convention activated via the
      `convention.<name>` layer source.
- [x] MDS034 itself does not emit new diagnostic
      types; all convention-driven diagnostics come
      from MDS041-MDS047.
- [x] Disabling MDS034 (`rules.markdown-flavor:
      false`) does not disable the rules a
      convention turned on; the preset has already
      been applied at config load.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
