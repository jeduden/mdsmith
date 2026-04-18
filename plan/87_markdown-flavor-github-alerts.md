---
id: 87
title: Flavor validation for GitHub Alerts
status: "🔲"
summary: >-
  Extend MDS034 to detect GitHub Alerts syntax
  (`> [!NOTE]` blockquote prefix) as a GFM-only
  feature with auto-fix that demotes the alert to
  a plain blockquote when the target flavor does
  not support it.
---
# Flavor validation for GitHub Alerts

Extends [plan 86](86_markdown-flavor-validation.md)
(MDS034, flavor validation). Add one feature —
GitHub Alerts — to the MDS034 feature enum.

Depends on: plan 86 lands first (provides the
dual parser, feature enum, fix pipeline).

## Goal

MDS034 flags `> [!NOTE]`-style alert blockquotes
when the target flavor is `commonmark` or
`goldmark`. `gfm` accepts them. Auto-fix demotes
the alert marker so the blockquote still renders
on non-GFM renderers.

## Context

GitHub added Alerts to GFM in December 2023
(see the `github.blog` changelog entry for
`new-syntax-for-alerts-on-github`). Five tokens
are recognized: `[!NOTE]`, `[!TIP]`,
`[!IMPORTANT]`, `[!WARNING]`, `[!CAUTION]`.
Obsidian callouts use the same prefix and accept
extra tokens, but only these five are standard
GFM.

On CommonMark / goldmark-default, the marker
renders as literal text inside a blockquote:

```markdown
> [!NOTE]
> Something to remember.
```

becomes a blockquote whose first line is the
literal string `[!NOTE]`. The author intended a
styled callout; the reader sees unstyled text
with the marker token visible inside the
blockquote. The failure is visible, not silent,
but the author's intent is still lost.

### Why not a generic container rule

The research spike evaluated four other
container syntaxes (Pandoc `:::` fenced divs,
MyST `:::{note}`, markdown-it-container, MkDocs
`!!! note`). None are mutually compatible and no
linter in the comparison covers them. GitHub
Alerts are the only variant with a standardized
spec, broad renderer support, and a clear
failure mode — so this plan covers them alone.

## Design

### Detection

GitHub Alerts need no new goldmark extension.
The syntax is a plain Blockquote. Its first
paragraph text must match
`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`
(case-sensitive per GFM).

Detection is an AST walk over `ast.Blockquote`
nodes on the dual parser's tree. The same walk
pattern the other 12 features use.

### Configuration

No new settings. GitHub Alerts join the existing
feature enum in
`internal/rules/markdownflavor/features.go` as
feature 13 (`GitHubAlerts`). Flavor support:

| Flavor     | GitHub Alerts |
|------------|---------------|
| commonmark | unsupported   |
| gfm        | supported     |
| goldmark   | unsupported   |

### Auto-fix

Remove the `[!TOKEN]` marker line, keeping the
rest of the blockquote intact:

```markdown
> [!NOTE]          > Something to
> Something to  →  > remember.
> remember.
```

If the alert marker is the only line in the
blockquote, remove the whole blockquote. The
marker line has no meaningful content once the
token is gone.

### Error message

`github alerts are not supported by {flavor}`

Severity: `warning`, matching the other MDS034
features.

## Tasks

1. Add `GitHubAlerts` to the feature enum in
   `internal/rules/markdownflavor/features.go`
2. Add flavor support table entry: supported in
   `gfm`, unsupported in `commonmark` and
   `goldmark`
3. Implement an AST detector that walks
   `ast.Blockquote` nodes and matches the five
   GFM tokens on the first paragraph child
4. Implement the fix: strip the marker line;
   drop the blockquote if empty afterward
5. Add unit tests: each of the five tokens,
   lower-case tokens (should not match), mixed
   content after the marker, marker as the only
   line
6. Add good/bad fixtures under
   `internal/rules/MDS034-markdown-flavor/alerts/`
7. Update the MDS034 README to list GitHub
   Alerts as the 13th feature

## Acceptance Criteria

- [ ] `flavor: commonmark` flags all five alert
      tokens
- [ ] `flavor: goldmark` flags all five alert
      tokens
- [ ] `flavor: gfm` accepts all five tokens
- [ ] `mdsmith fix` removes the marker line,
      preserves remaining blockquote content
- [ ] `mdsmith fix` removes the whole blockquote
      when the marker was its only line
- [ ] Lower-case or unknown tokens (e.g.
      `[!note]`, `[!INFO]`) produce no
      diagnostic — they are ordinary blockquote
      text
- [ ] Nested blockquotes are checked recursively
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
