---
id: '(int & >=1 & <=246) | (int & >=2601010000)'
title: 'string & != ""'
status: '"🔲" | "🔳" | "✅" | "⛔"'
summary: 'string | *""'
model: '"haiku" | "sonnet" | "opus" | *""'
depends-on: '[...int] | *[]'
---
<?require
filename: "[0-9]*_*.md"
?>

# ?

<!-- Plan conventions:
  - Work test-driven: write a failing test, make it
    pass, commit.
  - Plan files must pass `mdsmith check plan/`.
  - Use Markdown links for real repo paths in prose.
    Bare backticked paths are allowed in commands,
    code blocks, and placeholders.

  Plan ids:
  - The id is the minute-precision UTC creation
    time: `date -u +%y%m%d%H%M` (2606100533 is
    2026-06-10 05:33 UTC). Use it as both the
    frontmatter `id:` and the filename prefix.
  - Taken already? Add one minute and check again.
  - Ids 1-246 are the frozen legacy range. Never
    allocate max+1: the `id:` type above rejects
    new ids below 2601010000, so a sequential id
    fails `mdsmith check`.

  Status values:
  - 🔲 not started
  - 🔳 in progress
  - ✅ completed
  - ⛔ superseded (replaced by another plan)

-->

## ...

<?allow-empty-section?>

## Goal

One-sentence summary of what this task achieves and why
it matters.

## ...

<?allow-empty-section?>

## Tasks

1. First concrete step
2. Second concrete step
3. ...

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] Criterion described as observable behavior
- [ ] Another criterion
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues

## ...

<?allow-empty-section?>
