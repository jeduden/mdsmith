---
id: 209
title: Convention-per-file config under `.mdsmith/conventions/`
status: "🔳"
model: opus
depends-on: [208, 113]
summary: >-
  Reserve `.mdsmith/conventions/<name>.yaml` as the
  per-file home for user-defined conventions, mirroring
  the kind-files layout from plan 208. Each file holds
  one convention bundle (flavor + rules) keyed by
  basename.
---
# Convention-per-file config under `.mdsmith/conventions/`

## Goal

Lift each user convention out of `.mdsmith.yml`'s
`conventions:` block into a standalone YAML file under
`.mdsmith/conventions/<name>.yaml`. The basename is
the convention's name. The body is the full
`UserConvention` shape (flavor + rules). One file
describes everything about one convention.

## Background

Plan 113 added user-defined convention bundles under
the top-level `conventions:` key in `.mdsmith.yml`.
Plan 208 split kinds into `.mdsmith/kinds/<name>.yaml`
to isolate each kind's history. The same argument
applies to conventions: a project that defines its own
style preset shouldn't dirty `.mdsmith.yml` on every
rule edit.

The `.mdsmith/` tree already reserved
`conventions/` as a follow-up slot in plan 208's
design. This plan fills that slot.

## Non-Goals

- Removing inline `conventions.<name>:` from
  `.mdsmith.yml`. Inline stays as a first-class
  source (parallel to plan 208's stance on inline
  kinds).
- Built-in conventions (`portable`, `github`,
  `plain`). These stay compiled into the binary.
- Externalising the top-level `convention:` selector.
  That key remains in `.mdsmith.yml`.

## Design

### Directory layout

```text
.mdsmith.yml                       # unchanged
.mdsmith/
  kinds/                           # plan 208
    audit-log.yaml
  conventions/
    portable-strict.yaml           # one full convention
    long-form-docs.yaml
```

### Convention file shape

The body matches today's inline
`conventions.<name>:` shape — flavor + rules.

```yaml
# .mdsmith/conventions/portable-strict.yaml
flavor: commonmark
rules:
  line-length:
    max: 72
  no-bare-urls: true
```

The convention's name is the basename minus
extension. The basename must match
`[a-z][a-z0-9-]*` (same rule as kind files). One
convention per file. Subdirectories are rejected.

A name colliding between a file convention and an
inline convention is a config error naming both
sources. A name colliding with a built-in
(`portable`, `github`, `plain`) is also a config
error.

## Tasks

1. **`internal/config`**: add
   `discoverConventions(workspaceDir string)`
   modelled on `discoverKinds`. Unit test per
   rejection case.
2. **`internal/config`**: extend `Load` to merge
   file conventions into `cfg.Conventions` and
   error on name collisions (with inline or with
   built-ins).
3. **Provenance**: extend convention-layer source
   reporting so a user convention's defining file
   path surfaces in `kinds resolve` / `--explain`
   the same way kinds do.
4. **CLI**: extend any `conventions`-related
   output to print the defining-source path next
   to each convention it reports.
5. **Contract test** under
   `internal/integration/` mirroring plan 208's
   kind-file contract test.
6. **Docs**: add
   `docs/reference/convention-files.md`. Add a
   row to the cross-system boundaries table.
   Extend the relevant convention guide with a
   "split a convention into its own file"
   recipe.
7. **Repo migration**: out of scope here unless
   this repo defines a user convention worth
   splitting.

## Acceptance Criteria

- [ ] A convention at
      `.mdsmith/conventions/foo.yaml` with the
      same body as inline `conventions.foo:` emits
      byte-equal effective rules.
- [ ] A convention declared both in a file and
      inline errors naming both sources.
- [ ] A convention basename failing
      `[a-z][a-z0-9-]*` or a file in a subdir
      errors.
- [ ] A name collision with a built-in convention
      errors.
- [ ] `mdsmith kinds resolve <file>` prints the
      defining-source path on the convention
      layer when one is active.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes.
