---
title: mdsmith organization review
summary: >-
  Marketplace plugin that ships the
  `markdown-organization-review` skill for auditing
  and fixing Markdown file layout in an mdsmith
  repository.
---
# mdsmith organization review

A Claude Code skill that audits an mdsmith
repository. The skill targets structural problems
the built-in rules cannot see.

The checks cover hand-maintained indexes. They
cover similar files without a declared kind. They
cover a missing `.mdsmith.yml`. They cover
duplicated sections. They cover kinds that lack a
`path-pattern` or schema.

## Install

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-organization-review@mdsmith
/reload-plugins
```

## Run

Invoke from inside any mdsmith-aware repository.

```text
/markdown-organization-review audit
```

```text
/markdown-organization-review fix
```

Audit mode is read-only. Fix mode proposes
patches and waits for confirmation on each one.

## What it catches

See the skill's
[`SKILL.md`](skills/markdown-organization-review/SKILL.md)
for the full check list and severity rubric.
