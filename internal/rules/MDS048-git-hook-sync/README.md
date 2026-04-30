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
`pre-merge-commit install`) apply a fallback list of
`PLAN.md` and `README.md` when discovery is empty. The
fallback is install-only. Stale entries from it can still
trip the rule above.

The rule emits at most one diagnostic per repository. The
guard lives for the lifetime of the mdsmith process, so
linting many files in the same repo will not duplicate it.

## Fix

This rule is not auto-fixable. Git hook installation is a
side-effecting operation. To bring the hooks back into
sync, re-run the install commands. They pick up the
current set of files with generated directives:

```bash
# Re-install the merge driver and refresh .gitattributes
mdsmith merge-driver install

# Re-install the pre-merge-commit hook
mdsmith pre-merge-commit install
```

Both commands discover files with generated content and
overwrite the previous configuration.

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
- **Fixable**: no (re-run `mdsmith merge-driver install` /
  `mdsmith pre-merge-commit install`)
- **Implementation**: [source](../githooksync/rule.go)
- **Category**: meta
