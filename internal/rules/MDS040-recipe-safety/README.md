---
id: MDS040
name: recipe-safety
status: ready
description: >-
  Validate each build.recipes command for shell-safety at lint
  time; the rule never executes any binary.
---
# MDS040: recipe-safety

Validate each build.recipes command for shell-safety at lint
time; the rule never executes any binary.

## What it detects

MDS040 validates the `command` field of every recipe in
`build.recipes`. It applies six checks:

1. **Non-empty** — `command` must have at least one token.
2. **No shell interpreter** — the first token must not be
   `sh`, `bash`, `zsh`, `ksh`, `fish`, `/bin/sh`,
   `/bin/bash`, or similar.
3. **No shell operators in static parts** — tokens that are
   not purely a single `{param}` placeholder must not
   contain `&&`, `||`, `;`, `|`, `>`, `<`, `>>`, `2>`,
   `` ` ``, `$(`, or `${`.
4. **No fused placeholders** — a token containing two
   adjacent `{param}` patterns (e.g. `{a}{b}`) is rejected;
   the resulting concatenation at build time could produce
   unexpected values.
5. **No `..` in executable** — the first token must not
   contain a `..` path component.
6. **No unused params** (warning, not error) — every entry
   in `params.required` and `params.optional` must be
   referenced by at least one `{param}` token in `command`.

## Config

`build.recipes` is declared in the top-level `build:` section
of `.mdsmith.yml`:

```yaml
build:
  base-url: ""
  recipes:
    mermaid:
      command: "mmdc -i {input} --theme {theme}"
      body_template: "![{alt}]({output})"
      params:
        required: [input]
        optional: [theme]
```

MDS040 is enabled by default and configured implicitly from
`build:`. To disable:

```yaml
rules:
  recipe-safety: false
```

## Examples

### Good

```yaml
build:
  recipes:
    mermaid:
      command: "mmdc -i {input} --theme {theme}"
      params:
        required: [input]
        optional: [theme]
```

### Bad — shell interpreter

```yaml
build:
  recipes:
    audit:
      command: "bash audit.sh"
```

MDS040 reports: `recipe "audit": command uses shell interpreter
"bash" — use the direct binary`

### Bad — shell operator

```yaml
build:
  recipes:
    build:
      command: "make all && make install"
```

MDS040 reports: `recipe "build": command contains shell operator
"&&" — use a wrapper script`

### Bad — fused placeholders

```yaml
build:
  recipes:
    render:
      command: "tool {a}{b}"
      params:
        required: [a, b]
```

MDS040 reports: `recipe "render": command contains fused
placeholders "{a}{b}" — separate with a delimiter`

## Meta-Information

- **ID**: MDS040
- **Name**: `recipe-safety`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta
