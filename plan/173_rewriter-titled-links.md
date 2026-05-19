---
id: 173
title: Website rewriter tolerates titled links
status: "✅"
model: sonnet
depends-on: [170]
summary: >-
  Widen the website rewriter regexes to capture an
  optional link-title tail so a titled repo-relative
  link is rewritten and its title preserved instead of
  shipping as a dead path.
---
# Website rewriter tolerates titled links

## Goal

Stop `[x](../y.md "title")` from shipping as a dead
repo-relative path on the published site, closing gap
G6 from the
[link handling audit](../docs/research/links/README.md).

## Background

The audit found every rewrite regex in
[internal/release/website.go](../internal/release/website.go)
stops the path capture at whitespace (`\S+`), so a
Markdown title makes the link fail to match and pass
through unrewritten. The corpus has 0 titled links in
`docs/` today, so this is low-priority and pre-emptive,
but the gap is real and cheap to close.

## Tasks

1. [x] Add red tests for a titled link shipping a dead
   repo-relative path. Note: the original example
   `[x](../../../docs/a.md "t")` does not go red —
   `repoDocsLink`'s `([^)]*)` group already absorbs the
   title. The genuine G6 dead-path cases are the regexes
   that terminate with `\)` after a `\S+`/anchor capture
   (`repoNonPublishedLink`, `repoRuleLink`,
   `ruleReadmeLink`, `indexMdLink`, `ruleFixtureLink`,
   `ruleSiblingNonMDSLink`) and the `$`-anchored
   `repoRuleRefDef`; the tests cover those.
2. [x] Added a shared `linkTitleTail` (`(\s+"[^"]*")?`)
   appended to each affected regex and re-emitted by its
   replacement func/template. The `applyOutsideCode`
   guard is unchanged.
3. [x] Verified reference-def forms: the no-`$`
   definitions (`repoNonPublishedRefDef`,
   `repoPlanRefDef`, `ruleRefDefLink`) already let the
   title trail untouched; the `$`-anchored
   `repoRuleRefDef` was widened so it no longer breaks.
4. [x] Tests green; untitled-link and code-region
   regression guards still pass.

## Acceptance Criteria

- [x] A titled inline link is rewritten with its title
  preserved.
- [x] A titled reference definition is rewritten with
  its title preserved.
- [x] Code spans and fences are still untouched.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no issues.
