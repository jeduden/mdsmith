---
id: 113
title: User-defined Markdown conventions
status: "🔲"
summary: >-
  Extend the convention system from plan 112 with a
  top-level `conventions:` block in `.mdsmith.yml`.
  Teams can define their own convention inline, with
  the same shape as the built-in `portable`,
  `github`, and `plain`. No inheritance — each
  convention stands alone.
model: sonnet
---
# User-defined Markdown conventions

## Goal

Let a team define a convention inline in their
`.mdsmith.yml` without forking mdsmith. The three
built-in conventions cover common cases; this plan
covers everything else. A team that wants "GFM plus
no inline HTML plus dash bullets but allow
footnotes" gets there in seven lines of YAML.

## Background

### What plan 112 ships

[Plan 112](112_flavor-profiles.md) ships
conventions as a closed table baked into the binary.
The `Lookup(name string) (Convention, error)` helper
returns one of `portable`, `github`, or `plain`, or
a config error. Adding a fourth convention requires
a code change.

### Why inline and not external

Plan 112 considered three ways to open the system:
inline in `.mdsmith.yml`, separate convention files,
and inheritance via `extends:`. This plan ships
only the inline form. Reasons:

- The repo's `.mdsmith.yml` is already the source
  of truth for everything else mdsmith reads.
  Teams know where to look.
- Separate files add a path-resolution layer
  (relative? absolute? from which directory?) that
  pays off mostly when the convention is shared
  across repos. Within one repo, inline is shorter.
- `extends:` is genuinely useful but doubles the
  validation surface. Defer until a team asks.

A team that wants cross-repo reuse can copy the
seven lines, which is the same friction as managing
a separate file with no resolver bugs.

## Design

### Configuration

A new top-level `conventions:` key in
`.mdsmith.yml`, sibling to `kinds:`:

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

The `conventions:` map mirrors the built-in
convention table from plan 112. Each entry is a
`{ flavor, rules }` pair. The `rules` block uses
the same schema as the top-level `rules` block.

### Name collisions

User-defined names must not collide with the three
built-in convention names. Defining a
`conventions.portable` in `.mdsmith.yml` produces a
config error. The built-in names are reserved so
docs and tutorials keep meaning what they say.

### Resolution order

When the top-level `convention:` selector resolves:

1. Look up the name in user-defined `conventions:`
   first.
2. Fall back to the built-in table.
3. If neither matches, emit a config error listing
   both sets of names.

User conventions cannot shadow built-ins.
Collisions are rejected at parse time. The lookup
order is documented anyway so future maintainers
see the precedence explicitly.

### Validation

Each user-defined convention is validated at config
load:

- `flavor` must be one of `commonmark | gfm |
  goldmark`.
- Each key under `rules:` must name a registered
  rule.
- Each rule's settings must validate against that
  rule's existing schema (the same code path that
  validates a top-level `rules:` block).

Validation errors name the convention and the rule:
`convention "our-team" rule "no-inline-html":
unknown setting "allowed"`.

### Interaction with the user's `rules:` block

User-defined conventions apply as a base layer
beneath any top-level `rules:` overrides. This
matches how built-in conventions work in plan 112.
A team can set `convention: our-team` and then
override one rule in the top-level `rules:` block.
The override wins.

### Lookup signature

`Lookup(name)` from plan 112 grows a second arg —
the user-defined convention map from config. The
signature becomes `Lookup(name string, userConventions
map[string]Convention) (Convention, error)`. The
config loader reads `conventions:` once at startup
and passes the map through.

No new diagnostics. The check logic in MDS034 is
unchanged.

## Tasks

1. Add `Conventions` field to the top-level config
   struct in `internal/config/`.
2. Add YAML parsing for the `conventions:` block
   with the same schema as the built-in convention
   table.
3. Add reserved-name validation rejecting
   `portable`, `github`, and `plain` as user names.
4. Per-rule settings validation: reuse the
   `ApplySettings` validation path each rule
   already implements, called against an empty
   instance.
5. Extend `markdownflavor.Lookup` to consult the
   user map first, then the built-in table.
6. Wire the user map through the config loader to
   the lookup site.
7. Update `mdsmith kinds resolve` so user
   convention names appear with a `(user)` suffix
   to distinguish them from built-ins.
8. Add tests covering: a valid user convention, a
   name collision with a built-in, an unknown rule
   name, an invalid rule setting, and a top-level
   rules override on a user convention.
9. Document `conventions:` in the same place plan
   112 documents the built-ins.

## Acceptance Criteria

- [ ] `conventions:` block in `.mdsmith.yml`
      defines a named convention with `flavor:` and
      `rules:`.
- [ ] Top-level `convention: our-team` selects a
      user-defined convention and applies its rule
      presets.
- [ ] Defining `conventions.portable` (or `github` /
      `plain`) produces a config error naming the
      reserved name.
- [ ] An unknown convention name lists both
      built-in and user-defined options in the error
      message.
- [ ] Top-level `rules:` overrides win over user
      convention presets via deep-merge.
- [ ] Invalid rule names or settings inside a user
      convention produce a config error naming the
      convention and the rule.
- [ ] `mdsmith kinds resolve` distinguishes user
      conventions from built-ins in its output.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
