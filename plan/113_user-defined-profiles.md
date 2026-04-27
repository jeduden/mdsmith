---
id: 113
title: User-defined flavor profiles
status: "🔲"
summary: >-
  Extend the profile system from plan 112 with a
  top-level `profiles:` key in `.mdsmith.yml`. Teams
  can define their own profile inline, with the same
  shape as the built-in `portable`, `github`, and
  `plain`. No inheritance — each profile stands
  alone.
model: sonnet
---
# User-defined flavor profiles

## Goal

Let a team define a profile inline in their
`.mdsmith.yml` without forking mdsmith. The three
built-in profiles cover common cases; this plan
covers everything else. A team that wants "GFM plus
no inline HTML plus dash bullets but allow
footnotes" gets there in seven lines of YAML.

## Background

### What plan 112 ships

[Plan 112](112_flavor-profiles.md) ships profiles
as a closed table baked into the binary. The
`Lookup(name string) (Profile, error)` helper
returns one of `portable`, `github`, or `plain`,
or a config error. Adding a fourth profile requires
a code change.

### Why inline and not external

[Plan 112's review](#) considered three ways to
open the system: inline in `.mdsmith.yml`, separate
profile files, and inheritance via `extends:`. This
plan ships only the inline form. Reasons:

- The repo's `.mdsmith.yml` is already the source
  of truth for everything else mdsmith reads.
  Teams know where to look.
- Separate files add a path-resolution layer
  (relative? absolute? from which directory?) that
  pays off mostly when the profile is shared across
  repos. Within one repo, inline is shorter.
- `extends:` is genuinely useful but doubles the
  validation surface. Defer until a team asks.

A team that wants cross-repo reuse can copy the
seven lines, which is the same friction as managing
a separate file with no resolver bugs.

## Design

### Configuration

A new top-level `profiles:` key in `.mdsmith.yml`:

```yaml
profiles:
  our-team:
    flavor: gfm
    rules:
      no-inline-html:
        allow: [details, summary, kbd]
      list-marker-style:
        style: dash
      no-reference-style:
        allow-footnotes: true

rules:
  markdown-flavor:
    profile: our-team
```

The `profiles:` map mirrors the built-in profile
table from plan 112. Each entry is a
`{ flavor, rules }` pair. The `rules` block uses
the same schema as the top-level `rules` block.

### Name collisions

User-defined names must not collide with the three
built-in profile names. Defining a `profiles.portable`
in `.mdsmith.yml` produces a config error. The
built-in names are reserved so docs and tutorials
keep meaning what they say.

### Resolution order

When `markdown-flavor.profile` resolves:

1. Look up the name in user-defined `profiles:`
   first.
2. Fall back to the built-in table.
3. If neither matches, emit a config error listing
   both sets of names.

User profiles cannot shadow built-ins. Collisions
are rejected at parse time. The lookup order is
documented anyway so future maintainers see the
precedence explicitly.

### Validation

Each user-defined profile is validated at config
load:

- `flavor` must be one of `commonmark | gfm |
  goldmark`.
- Each key under `rules:` must name a registered
  rule.
- Each rule's settings must validate against that
  rule's existing schema (the same code path that
  validates a top-level `rules:` block).

Validation errors name the profile and the rule:
`profile "our-team" rule "no-inline-html": unknown
setting "allowed"`.

### Interaction with the user's `rules:` block

User-defined profiles apply as a base layer beneath
any top-level `rules:` overrides. This matches how
built-in profiles work in plan 112. A team can set
`profile: our-team` and then override one rule in
the top-level `rules:` block. The override wins.

### MDS034 changes

`Lookup(name)` from plan 112 grows a second arg —
the user-defined profile map from config. The
signature becomes
`Lookup(name string, userProfiles map[string]Profile)
(Profile, error)`. The config loader reads
`profiles:` once at startup and passes the map
through.

No new diagnostics. The check logic in MDS034 is
unchanged.

## Tasks

1. Add `Profiles` field to the top-level config
   struct in `internal/config/`.
2. Add YAML parsing for the `profiles:` block with
   the same schema as the built-in profile table.
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
7. Update `mdsmith kinds --explain` so user profile
   names appear with a `(user)` suffix to
   distinguish them from built-ins.
8. Add fixture tests under
   `internal/rules/MDS034-markdown-flavor/profiles/user/`
   covering: a valid user profile, a name collision
   with a built-in, an unknown rule name, an
   invalid rule setting, and a top-level rules
   override on a user profile.
9. Document `profiles:` in the same place plan 112
   documents the built-ins.

## Acceptance Criteria

- [ ] `profiles:` block in `.mdsmith.yml` defines a
      named profile with `flavor:` and `rules:`.
- [ ] `markdown-flavor.profile: our-team` selects a
      user-defined profile and applies its rule
      presets.
- [ ] Defining `profiles.portable` (or `github` /
      `plain`) produces a config error naming the
      reserved name.
- [ ] An unknown profile name lists both built-in
      and user-defined options in the error
      message.
- [ ] Top-level `rules:` overrides win over user
      profile presets via deep-merge.
- [ ] Invalid rule names or settings inside a user
      profile produce a config error naming the
      profile and the rule.
- [ ] `mdsmith kinds --explain` distinguishes user
      profiles from built-ins in its output.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
