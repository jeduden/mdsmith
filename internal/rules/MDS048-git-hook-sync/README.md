---
id: MDS048
name: git-hook-sync
status: ready
description: Git hooks must be in sync with files containing generated content directives.
---
# MDS048: git-hook-sync

Git hooks must be in sync with files containing generated
content directives.

## Rationale

When markdown files contain generated sections (`<?catalog?>`,
`<?include?>`, `<?toc?>`), they need special handling during
git merges to regenerate content and avoid conflicts. The
mdsmith git hooks automate this process.

This rule detects when:

- The merge-driver assignments in `.gitattributes` cover a
  different set of files than those currently containing
  directives
- The pre-merge-commit hook is configured for a different set
  of files than those currently containing directives

## Settings

This rule has no configuration settings beyond enabling/disabling it.

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

1. Scans the repository for markdown files containing
   generated section directives
2. If `merge.mdsmith.driver` is registered in git config,
   compares the file list in `.gitattributes` (lines of the
   form `<file> merge=mdsmith`) against the discovered files
3. Reads the pre-merge-commit hook (when it carries the
   mdsmith marker) and compares the file list extracted from
   `mdsmith fix --` lines against the discovered files
4. Reports a warning if either source is out of sync

If discovery is empty:

- If neither managed source lists files, the rule is silent.
- If a managed source still lists files, the rule reports
  those entries as stale (drift against an empty set).

The install commands (`mdsmith merge-driver install` and
`mdsmith pre-merge-commit install`) apply a fallback list of
`PLAN.md` and `README.md` when discovery is empty. The
fallback is install-only. Stale entries from it can still
trip the rule above.

Drift is computed from repository-wide discovery of files
containing generated section directives. When `mdsmith fix`
applies the `.gitattributes` fix, it regenerates that file
once per repository run from the current discovered file
list. Check() reports drift at most once per repository per
process to avoid noisy output when checking many files.

## Fix

This rule is **partially auto-fixable**:

- **`.gitattributes`** is auto-fixed by `mdsmith fix` when
  the merge driver is registered (`merge.mdsmith.driver` in
  git config). The fixer regenerates `.gitattributes` with
  the current discovered file list, preserving header
  comments.
- **Pre-merge-commit hook** is not auto-fixed (it is an
  executable script). You must manually re-run:

```bash
mdsmith pre-merge-commit install
```

### Why Auto-Fix .gitattributes?

`.gitattributes` is a tracked file (not system configuration,
not executable) and should be auto-fixable like other
content issues. This enables build systems running
`mdsmith fix` to automatically resolve drift.

The pre-merge-commit hook remains manual-only because
modifying executable files during automated fixes could be
surprising or unsafe.

When `mdsmith fix` updates `.gitattributes`, the fixer also
runs `git add -- .gitattributes`. The regenerated file then
lands in the index next to the markdown files the hook stages.
A merge commit produced by the pre-merge-commit hook flow
includes both. If staging fails (for example, the index is
locked), the on-disk fix is still applied. You can stage the
file yourself in that case.

### Manual Installation

If the merge driver is not registered, or to update the
pre-merge-commit hook, run:

```bash
# Register merge driver and regenerate .gitattributes
mdsmith merge-driver install

# Re-install the pre-merge-commit hook
mdsmith pre-merge-commit install
```

Both commands pick up the current set of files with
generated directives.

## Examples

### Good

Hooks match the files with generated content:

```markdown
# README.md
<?catalog?>
- [File 1](file1.md)
<?/catalog?>
```

```text
# .gitattributes contains:
README.md merge=mdsmith

# git config (local) contains:
merge.mdsmith.driver=mdsmith merge-driver run %O %A %B %P

# .git/hooks/pre-merge-commit contains:
# mdsmith merge-driver pre-merge-commit hook
mdsmith fix -- 'README.md'
git add -- 'README.md'
```

### Bad

`.gitattributes` is configured for `PLAN.md` but the file
that actually contains a directive is `test.md`:

```markdown
# test.md
<?catalog?>
- [File 1](file1.md)
<?/catalog?>
```

```text
# .gitattributes contains:
PLAN.md merge=mdsmith
```

**Diagnostic**: `merge-driver assignments in .gitattributes
are out of sync (has: PLAN.md, should have: test.md)`

## Meta-Information

- **ID**: MDS048
- **Name**: `git-hook-sync`
- **Status**: ready
- **Default**: disabled
- **Fixable**: partial (`.gitattributes` is auto-fixed;
  hook requires manual `mdsmith pre-merge-commit install`)
- **Implementation**: [source](../githooksync/rule.go)
- **Category**: meta
