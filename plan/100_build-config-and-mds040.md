---
id: 100
title: build config block and MDS040 recipe-safety rule
status: "✅"
summary: >-
  Add a `build:` config section that declares `base-url`
  and user-defined `recipes`. MDS040 lints each recipe's
  `command` field: no shell interpreter as first token,
  no shell operators in static parts, no fused
  `{param}` placeholders.
model: sonnet
---
# build config block and MDS040 recipe-safety rule

## Goal

Introduce the `build:` config schema. User-defined
recipes are declared in `.mdsmith.yml`. MDS040
validates each recipe `command` for shell-safety at
lint time, before any directive or subcommand depends
on the schema.

## Context

Plans 101 and 102 both read `build.recipes` to resolve
recipe names and param schemas. This plan delivers the
config layer first so those plans have a stable
foundation. MDS040 is a pure linter — it reads the
parsed config and emits diagnostics; it never runs
any external tool.

## Design

### Config schema

New top-level key `build:` in `.mdsmith.yml`:

```yaml
build:
  base-url: ""        # joined to path-only `url` params
  recipes:            # user-defined recipes (map)
    mermaid:
      command: "mmdc -i {input} -o {output}"
      body_template: "![{alt}]({output})"
      params:
        required: [input]
        optional: [theme]
    api-spec:
      command: "redocly bundle {input} -o {output}"
      body_template: "[API spec]({output})"
      params:
        required: [input]
```

Both keys are `omitempty`; a config without `build:`
continues to parse and `check` without error.

#### `build.base-url`

String. When a `url` param in a `<?build?>` directive
is a path-only value (starts with `/`), `base-url` is
prepended at build time. Has no effect on lint.

#### `build.recipes`

Map from recipe name to a recipe declaration:

| Field             | Required | Description                                                                 |
|-------------------|----------|-----------------------------------------------------------------------------|
| `command`         | yes      | Argv template; `{param}` tokens expand at build time                        |
| `body_template`   | no       | Markdown body rendered by `mdsmith fix`; defaults to `[{output}]({output})` |
| `params.required` | no       | Param names the directive must supply                                       |
| `params.optional` | no       | Param names the directive may supply                                        |

`{param}` tokens in `command` expand to individual
`os/exec` arguments — no shell. An unknown `{param}`
token (one not in `required` or `optional`) is a
config error.

`{alt}` and `{output}` are reserved template variables
available in `body_template`. `{alt}` defaults to
`"{recipe} output: {output}"`. They must not appear
in `command`.

### Rule: MDS040 (recipe-safety)

- ID: `MDS040`
- Name: `recipe-safety`
- Category: `meta`
- Default: enabled
- Fixable: no
- Target: `.mdsmith.yml`

MDS040 validates every `command` in `build.recipes`:

1. **Non-empty** — `command` must have at least one
   token.
2. **No shell interpreter** — the first token must not
   be `sh`, `bash`, `zsh`, `ksh`, `fish`, `/bin/sh`,
   `/bin/bash`, or similar.
3. **No shell operators in static parts** — each token
   that is not entirely a single `{param}` placeholder
   must contain none of: `&&`, `||`, `;`, `|`, `>`,
   `<`, `>>`, `2>`, `` ` ``, `$(`, `${`.
4. **No fused placeholders** — a single token that
   contains two adjacent `{param}` patterns (e.g.
   `{a}{b}`) is rejected; the resulting concatenation
   at build time could produce unexpected values.
5. **No `..` in executable** — the first token must
   not contain a `..` path component.
6. **No unused params** (warning, not error) — every
   entry in `params.required` and `params.optional`
   must be referenced by at least one `{param}`
   token in `command`. Catches typos before they
   waste a build.

`command` is split on whitespace into tokens for
validation (the same split used at build time by plan
102). MDS040 does not execute any binary.

Example diagnostics:

```text
.mdsmith.yml:5:5: recipe "audit": command uses shell
interpreter "bash" — use the direct binary (MDS040)

.mdsmith.yml:9:5: recipe "audit": command contains
shell operator "&&" — use a wrapper script (MDS040)

.mdsmith.yml:13:5: recipe "render": command contains
fused placeholders "{a}{b}" — separate with a
delimiter (MDS040)
```

## Tasks

1. Add `BuildConfig`, `RecipeCfg`, and `ParamCfg`
   structs to `internal/config/`. Both `build:` keys
   are `omitempty`. Validate that `{param}` tokens in
   `command` are all declared in `required` or
   `optional`; emit a config error for unknowns.
   Validate that `{alt}` and `{output}` do not appear
   in `command`.
2. Add MDS040 (`recipe-safety`) implementation in
   `internal/rules/recipesafety/` (Go package).
   Implement the six checks above (the five
   safety checks plus the unused-param warning).
   Diagnostic messages include the recipe name
   and the offending string. Not fixable.
3. Add `good/` and `bad/` fixtures for MDS040
   under `internal/rules/MDS040-recipe-safety/`.
4. Wire MDS040 into `cmd/mdsmith/main.go`.
5. Document MDS040 in
   `internal/rules/MDS040-recipe-safety/README.md`.

## Acceptance Criteria

- [x] `build:` parses correctly with both keys absent,
      one present, or both present
- [x] A recipe with `command: "mmdc -i {input} -o {output}"`,
      `params.required: [input]`, and
      `params.optional: [theme]` round-trips through
      config parse without error
- [x] A `{param}` token in `command` that is not in
      `required` or `optional` produces a config error
- [x] `{alt}` or `{output}` appearing in `command`
      produces a config error
- [x] MDS040 flags a recipe whose `command` starts with
      `bash`, `sh`, `/bin/bash`, or `/bin/sh`
- [x] MDS040 flags a recipe whose `command` contains a
      shell operator token (`&&`, `||`, `;`, `|`, `>`, etc.)
- [x] MDS040 flags a recipe whose `command` token
      contains fused adjacent `{param}` placeholders
      (e.g. `{a}{b}`)
- [x] MDS040 flags a recipe whose executable token
      contains `..`
- [x] MDS040 warns (not errors) when a recipe declares
      a `params.required` or `params.optional` entry
      that no `{param}` token in `command` references
- [x] MDS040 passes a clean command like
      `mmdc -i {input} -o {output}`
- [x] A config with no `build:` key passes MDS040
      without error
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
