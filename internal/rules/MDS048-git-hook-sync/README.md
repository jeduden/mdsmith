---
id: MDS048
name: git-hook-sync
status: ready
description: Git hooks (merge-driver and pre-merge-commit) must be in sync with files containing generated content directives.
---
# MDS048: git-hook-sync

Git hooks (merge-driver and pre-merge-commit) must be in sync with files containing generated content directives.

## Rationale

When markdown files contain generated sections (<?catalog?>, <?include?>, <?toc?>), they need special handling during git merges to regenerate content and avoid conflicts. The mdsmith git hooks automate this process.

This rule detects when:
- The merge-driver hook is installed but configured for different files than those currently containing directives
- The pre-merge-commit hook is installed but configured for different files than those currently containing directives

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
1. Scans the repository for markdown files containing generated section directives
2. Checks if merge-driver entries in `.git/config` match the discovered files
3. Checks if the pre-merge-commit hook processes the discovered files
4. Reports a warning if either hook is out of sync

The rule only checks once per run (when processing README.md or PLAN.md) to avoid duplicate diagnostics.

## Fix

This rule is marked as fixable but does not automatically fix the issue. To fix out-of-sync hooks:

```bash
# Update merge-driver for all files
mdsmith merge-driver install

# Update pre-merge-commit hook for all files
mdsmith pre-merge-commit install
```

Both commands will discover files with generated content and update the hooks accordingly.

## Examples

### Good

Hooks are installed and match the files with generated content:

```markdown
# README.md
<?catalog?>
- [File 1](file1.md)
<?/catalog?>
```

```bash
# .git/config contains:
[merge "mdsmith-README.md"]
    driver = mdsmith merge-driver -- 'README.md' %O %A %B %P

# .git/hooks/pre-merge-commit contains:
mdsmith fix -- 'README.md'
git add -- 'README.md'
```

### Bad

Hooks are installed but for different files:

```markdown
# test.md (has generated content)
<?catalog?>
- [File 1](file1.md)
<?/catalog?>
```

```bash
# .git/config contains merge driver for PLAN.md (wrong file)
[merge "mdsmith-PLAN.md"]
    driver = mdsmith merge-driver -- 'PLAN.md' %O %A %B %P
```

**Diagnostic**: `git-hook-sync: merge-driver hook is out of sync (has: PLAN.md, should have: test.md)`

## Meta-Information

- **ID**: MDS048
- **Name**: `git-hook-sync`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes (but requires manual reinstall)
- **Implementation**: [source](./rule.go)
- **Category**: meta
