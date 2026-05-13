---
title: mdsmith audit plugin
summary: >-
  Marketplace plugin that ships the
  `markdown-audit` skill for auditing and fixing
  Markdown file layout in an mdsmith repository.
---
# mdsmith audit plugin

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
/plugin install mdsmith-audit@mdsmith
/reload-plugins
```

## Run

Invoke from inside any mdsmith-aware repository.

```text
/markdown-audit audit
```

```text
/markdown-audit fix
```

Audit mode is read-only. Fix mode proposes
patches and waits for confirmation on each one.

## What it catches

See the skill's
[`SKILL.md`](skills/markdown-audit/SKILL.md)
for the full check list and severity rubric.
