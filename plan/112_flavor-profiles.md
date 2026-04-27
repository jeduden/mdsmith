---
id: 112
title: Flavor profiles refactor
status: "🔲"
summary: >-
  Refactor MDS034 from "extension support detector"
  to a flavor orchestrator. Add a profile concept
  that bundles extension policy with style policy.
  Three profiles ship: portable (CommonMark
  everywhere), github (GFM on github.com), and plain
  (Markdown that survives `cat`).
model: opus
---
# Flavor profiles refactor

## Goal

Make `flavor:` the single knob for "what subset of
Markdown does this project use." Today MDS034 only
answers which extensions a renderer supports. This
plan extends it to also answer which CommonMark
constructs a team allows and which style choices
they pin. One config key, one mental model.

## Background

### Today's flavor model

MDS034
([internal/rules/markdownflavor/](../internal/rules/markdownflavor/))
defines three flavors: `commonmark`, `gfm`,
`goldmark`. Each declares which of 12 extension
features it supports. The rule emits diagnostics
when the configured flavor does not support a
feature the file uses.

Settings:

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

Seven new rules cover MDS039 through MDS045. Each
restricts a different CommonMark ambiguity. Today
they would each be enabled and configured
separately:

```yaml
rules:
  no-inline-html: true
  no-reference-style: { allow-footnotes: false }
  emphasis-style: { bold: asterisk, italic: underscore }
  horizontal-rule-style: { style: dash, length: 3 }
  list-marker-style: { style: dash }
  ordered-list-numbering: { style: sequential, start: 1 }
  ambiguous-emphasis: { max-run: 2 }
  markdown-flavor: { flavor: commonmark }
```

Eight separate knobs to land on a strict
Markdown setup. The user request is to collapse
these into one.

### Why "profile" and not "flavor"

A flavor today corresponds to a renderer. Adding
`portable` as a flavor would suggest there is
a renderer by that name, which there is not. The
plan introduces **profiles** as a layer above
flavors. A profile names a flavor *plus* a set of
rule presets. Selecting a profile applies both.

This keeps the existing `flavor:` enum honest
(`commonmark | gfm | goldmark`) and adds a separate
`profile:` field for opinionated bundles.

## Design

### Configuration

```yaml
rules:
  markdown-flavor:
    flavor: commonmark        # renderer grammar
    profile: portable   # opinionated bundle
```

Both fields are optional. `flavor` defaults to
`commonmark`. `profile` defaults to empty.

When a profile is set, it implies a flavor. Setting
both is allowed only when they agree; a mismatch
(e.g. `flavor: gfm` with a profile that requires
`commonmark`) is a config error.

### Built-in profiles

Three profiles ship initially. Each names a target.

- **`portable`** — Markdown that renders the same in
  every CommonMark parser. `flavor: commonmark`, all
  of MDS039-MDS045 enabled with recommended defaults
  (no inline HTML, no reference links, asterisk
  bold, underscore italic, dash HR, dash list
  marker, sequential ordered numbering, max emphasis
  run 2).
- **`github`** — Markdown that renders well on
  github.com. `flavor: gfm`, MDS039 enabled with
  `<details>` and `<summary>` in the allowlist,
  MDS040 and MDS043 enabled with defaults, the rest
  disabled. Targets teams that use GFM for GitHub
  rendering but still want consistent emphasis and
  bullet style.
- **`plain`** — Markdown that survives `cat`. The
  rendered output should look about the same as the
  source viewed in a plaintext reader. Same
  activations as `portable` today, plus
  `allow-comments: false` on MDS039 (HTML comments
  leak through as `<!-- ... -->` in plaintext).

The `plain` profile sits close to `portable` in
this plan. A truly plaintext-faithful profile would
need extra rules that do not exist yet:

- **no-emphasis** — forbid `*` and `_` runs entirely.
  In plaintext readers they appear as literal
  characters around the word, which is noise.
- **no-fenced-code** — require indented code blocks
  rather than fenced ones, since the fence delimiters
  show up literally.
- **prefer-bare-urls** — invert MDS012 so
  `https://example.com` is preferred over
  `[text](url)`, since the latter renders as
  `[text](url)` literally in plaintext.

These three rules are out of scope for this plan
and would land as plans 113-115. When they ship,
the `plain` profile gains them automatically and
diverges from `portable`.

Profiles are declared in
`internal/rules/markdownflavor/profiles.go` as a
table: profile name → flavor + rule preset map.

### How preset application works

Profiles do not re-implement the rule logic. They
push settings into the existing rule registry
during config load. The flow:

1. Config loader reads `markdown-flavor.profile`.
2. If non-empty, it looks up the profile table.
3. For each rule named in the profile preset, the
   loader applies the preset *as a base layer*
   under any user override.

This piggybacks on the existing deep-merge config
([plan 97](97_deep-merge-config.md)). User overrides
still win — a team can set
`profile: portable` and then opt back in to
inline HTML for `<sub>` and `<sup>` by setting
`rules.no-inline-html.allow: [sub, sup]`. The
preset provides the floor.

### MDS034 runtime changes

MDS034's `Check()` does not gain new diagnostics.
Its job stays the same. Detect unsupported
extensions for the configured flavor. The profile
mechanism is a config-layer concern. The reasons:

- Each rule already owns its detection logic.
  Duplicating into MDS034 would create two places
  that emit the same diagnostic.
- Tests for MDS039-MDS045 should not depend on
  MDS034 being enabled.
- Disabling MDS034 should not silently disable the
  style rules a profile turned on.

What MDS034 does gain: a `profile` field in its
`ApplySettings` handler, plus validation that
`flavor` and `profile` agree.

### Failure modes

- Unknown profile name → config error at load time
  with the list of valid names.
- Profile/flavor disagreement → config error naming
  both values.
- Profile names a rule that no longer exists →
  config error naming the rule. Profiles are
  versioned with the codebase; this case only
  fires after a removal.

## Tasks

1. Add `profiles.go` in
   `internal/rules/markdownflavor/` with the
   profile table and a `Lookup(name string)
   (Profile, error)` helper.
2. Extend `ApplySettings` on MDS034 to accept
   `profile` and validate against `flavor`.
3. Wire profile-preset application into the config
   loader as a base layer beneath user overrides.
   Reuse the deep-merge path from plan 97.
4. Add a `profile` field to the kinds resolution
   output ([plan 95](95_kind-rule-resolution-cli.md))
   so `mdsmith kinds --explain` shows which rules a
   profile turned on.
5. Document the three built-in profiles in
   `docs/guides/directives/enforcing-structure.md`
   or a new `docs/reference/profiles.md`.
6. Add fixture tests under
   `internal/rules/MDS034-markdown-flavor/profiles/`
   covering each profile applied to a passing file
   and a failing file.

## Acceptance Criteria

- [ ] Setting `profile: portable` enables
      MDS039-MDS045 with documented preset values.
- [ ] Setting `profile: github` enables MDS039,
      MDS040, MDS043 with their documented presets
      and leaves the rest disabled.
- [ ] Setting `profile: plain` enables
      MDS039-MDS045 with the portable presets plus
      `allow-comments: false` on MDS039.
- [ ] User overrides win over profile presets via
      deep-merge (e.g. extending the inline-HTML
      allowlist).
- [ ] Setting both `flavor: gfm` and a profile that
      requires `commonmark` produces a config error.
- [ ] Setting an unknown profile name produces a
      config error naming the field.
- [ ] `mdsmith kinds --explain` reports which rules
      a profile activated and with which settings.
- [ ] MDS034 itself does not emit new diagnostic
      types; all profile-driven diagnostics come
      from MDS039-MDS045.
- [ ] Disabling MDS034 does not disable the rules a
      profile turned on (the preset has already
      been applied at config load).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
