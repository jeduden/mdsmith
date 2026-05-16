---
id: MDS048
name: git-hook-sync
status: ready
description: Git artifacts must match the canonical glob-based template derived from .mdsmith.yml.
category: structural
nature: structure
maintainability: null
---
# MDS048: git-hook-sync

Git artifacts must match the canonical glob-based template
derived from .mdsmith.yml.

The `.gitattributes` managed block and the
pre-merge-commit hook must match a canonical
template. That template is computed from the
project's `.mdsmith.yml` ignore patterns. Editing
`.mdsmith.yml` keeps both git artifacts in sync.

One limitation: ignore patterns that cannot appear
verbatim in `.gitattributes` are dropped from the
managed block. Negation patterns starting with `!`
and patterns containing whitespace fall in this
bucket. `mdsmith fix` itself still honors them, so
the merge-driver scope can diverge from `mdsmith
fix`'s scope when the `ignore:` list relies on those
forms.

## Rationale

Markdown files can contain generated sections like
`<?catalog?>`, `<?include?>`, and `<?toc?>`. They need
special handling during git merges to regenerate content
and avoid conflicts. Two artifacts cooperate:

- `.gitattributes` assigns the `mdsmith` merge driver to
  markdown files. The managed block uses globs (e.g.
  `*.md merge=mdsmith`) plus exclude overrides
  (`<pattern> -merge`) so the assignment scope tracks
  `.mdsmith.yml` ignore patterns rather than enumerating
  individual files.
- The pre-merge-commit hook re-runs `mdsmith fix .` once
  every per-file merge has resolved, so generated sections
  reflect the final merged state. The hook script is
  glob-driven (no embedded file list), so its scope tracks
  the same ignore patterns automatically.

This rule detects when either artifact drifts from the
canonical template.

## Settings

This rule has no configuration settings beyond
enabling/disabling it.

## Config

Enable (default: disabled):

```yaml
rules:
  git-hook-sync: true
```

Disable:

```yaml
rules:
  git-hook-sync: false
```

## How It Works

The rule:

1. Reads `.mdsmith.yml` ignore patterns and computes the
   canonical glob set: include patterns (default: `*.md`,
   `*.markdown`) followed by an exclude pattern for each
   `ignore:` entry.
2. If `merge.mdsmith.driver` is registered in git config,
   reads `.gitattributes` and compares the BEGIN/END
   managed block against the canonical render.
3. Reads the pre-merge-commit hook (when it carries the
   mdsmith marker) and compares the script against the
   canonical glob-based template.
4. Reports a warning if either artifact drifts.

`.gitattributes` does not support negation patterns
(`!*.md` is a syntax error there). The managed block
exploits last-match-wins instead: include lines come first,
then `<exclude> -merge` lines override them for matching
paths.

## Fix

This rule is **partially auto-fixable**:

- **`.gitattributes`** is auto-fixed by `mdsmith fix` when
  the merge driver is registered (`merge.mdsmith.driver`
  in git config). The fixer rewrites the managed block
  with the canonical globs, preserving any non-mdsmith
  entries surrounding it.
- **Pre-merge-commit hook** is not auto-fixed (it is an
  executable script). You must manually re-run:

```bash
mdsmith pre-merge-commit install
```

### Why Auto-Fix `.gitattributes`?

`.gitattributes` is a tracked file. It is not system
configuration, and not executable. So it should be
auto-fixable like other content issues. Build systems
running `mdsmith fix` reconcile drift automatically.

The pre-merge-commit hook remains manual-only because
modifying executable files during automated fixes could be
surprising or unsafe.

When `mdsmith fix` updates `.gitattributes`, the fixer also
runs `git add -- .gitattributes`. The regenerated file then
lands in the index next to the markdown files the hook
stages, so a merge commit produced by the
pre-merge-commit hook flow includes both. If staging fails
(for example, the index is locked by another git process),
the on-disk fix is still applied and the failure is
recorded. `Check()` keeps emitting a "staging failed"
diagnostic until a later `Fix()` call re-runs `git add`
successfully.

### Manual Installation

To register the merge driver, write the canonical
`.gitattributes` block, and install the hook:

```bash
mdsmith merge-driver install
mdsmith pre-merge-commit install
```

Both commands write the canonical glob-based content from
the project's `.mdsmith.yml`, so re-running them is
idempotent.

## Examples

### Good

`.mdsmith.yml` ignores test fixtures, and `.gitattributes`
mirrors that exactly:

```yaml
# .mdsmith.yml
ignore:
  - "demo/**"
  - "vendor/**"
```

```text
# .gitattributes
# BEGIN mdsmith merge-driver
*.md merge=mdsmith
*.markdown merge=mdsmith
demo/** -merge
vendor/** -merge
# END mdsmith merge-driver
```

### Bad

`.gitattributes` still uses the old per-file format from a
pre-glob install:

```text
# BEGIN mdsmith merge-driver
README.md merge=mdsmith
docs/index.md merge=mdsmith
# END mdsmith merge-driver
```

**Diagnostic**: `.gitattributes managed block is out of
sync`. The message lists the installed include and exclude
patterns and the canonical patterns the rule expected.

## Meta-Information

- **ID**: MDS048
- **Name**: `git-hook-sync`
- **Status**: ready
- **Default**: disabled
- **Fixable**: partial (`.gitattributes` is auto-fixed;
  hook requires manual `mdsmith pre-merge-commit install`)
- **Implementation**: [source](../githooksync/rule.go)
- **Category**: structural
