---
id: 114
title: MDS034 message clarity and flavor-vs-rule docs
status: "✅"
summary: >-
  Reword MDS034 diagnostics so they don't sound like
  parse failures, drop the dead Verb() helper, and
  add a concept doc that distinguishes "flavor"
  (what the renderer recognizes) from "rule" (what
  the team prefers). Link the doc from the MDS034
  README and the conventions reference.
model: sonnet
---
# MDS034 message clarity and flavor-vs-rule docs

## Goal

Two things. Reword MDS034 so its messages don't
sound like parse failures. Write down the line
between "flavor" and "rule" so future readers stop
asking what the difference is.

## Background

MDS034 today emits messages like `"tables are not
supported by commonmark"`. That reads like a parse
failure. Every flavor MDS034 checks against does
parse the source. The renderer simply doesn't
recognize the syntax as the named feature.

Concrete examples of the same point. GFM tables in
CommonMark render as paragraphs of literal pipes.
`[!NOTE]` GitHub alerts render as a normal
blockquote. Heading IDs `{#id}` render as literal
trailing text inside the heading.

The same review surfaced a recurring confusion.
What is the line between flavor (MDS034's domain)
and rule (every other MDS0xx)? Both reject syntax,
both have settings, both emit warnings. The line is
real — flavors are properties of a renderer, rules
are properties of a team — but it is not written
down anywhere readers can find.

## Design

### Diagnostic format

Change the `fmt.Sprintf` template in the
[markdownflavor rule](../internal/rules/markdownflavor/rule.go)
from:

```text
%s %s not supported by %s
```

(feature name, verb, flavor — verb is "is" or "are"
depending on the feature) to:

```text
%s does not interpret %s as a feature
```

(flavor, feature name). Examples:

| Before                                        | After                                                    |
|-----------------------------------------------|----------------------------------------------------------|
| `tables are not supported by commonmark`      | `commonmark does not interpret tables as a feature`      |
| `inline math is not supported by commonmark`  | `commonmark does not interpret inline math as a feature` |
| `footnotes are not supported by gfm`          | `gfm does not interpret footnotes as a feature`          |
| `github alerts are not supported by goldmark` | `goldmark does not interpret github alerts as a feature` |

The new format drops the `Feature.Verb()` helper
(its only caller was the old template). Removing it
keeps the package small and prevents future
divergence.

### Fixture updates

The
[bad fixtures](../internal/rules/MDS034-markdown-flavor/bad/)
encode the old diagnostic text in their YAML
frontmatter `diagnostics:` block. Each file gets
its expected message rewritten.

Three `profile-*` fixtures landed alongside this
plan during plan 112's first iteration. They were
removed in the later convention rename, so the
final fixture count is eighteen.

### Concept doc

A new
[concepts doc](../docs/background/concepts/flavor-rule-convention-kind.md)
covers, in this order:

1. **TL;DR table** — flavor vs rule across four
   axes: source of truth, who owns it, what it
   answers, what its diagnostic asserts. (The doc
   later grew to cover convention and kind too;
   see the implementation note below.)
2. **Why both exist** — flavors are external (a spec
   or a renderer's behavior); rules are internal
   (your team's preference among equally-valid
   forms). Examples of each.
3. **The fuzzy middle** — features that "parse but
   don't render": tables/task lists/alerts under
   CommonMark all parse fine; the renderer just
   doesn't treat the syntax as the named feature.
   Bare URLs are the cleanest case where flavor
   (`flavor: commonmark` won't auto-link them) and
   rule (`no-bare-urls` rejects them on style
   grounds) overlap legitimately.
4. **Conventions as the bridge** — short pointer
   to the
   [conventions reference](../docs/reference/conventions.md)
   for how the two get bundled together for users.

Length target: under 100 lines. The doc is reference
material, not a tutorial.

Implementation note: the concept doc shipped first
under the working name `flavor-vs-rules.md` with
just two concepts. After the convention rename
(see [plan 112](112_flavor-profiles.md)) it was
renamed to `flavor-rule-convention-kind.md` and
expanded to four. The references in this plan have
been updated to match what shipped.

### Links

- The new concept doc gets a one-paragraph mention
  in the MDS034 README (after the lead) and in the
  conventions reference at
  [docs/reference/conventions.md](../docs/reference/conventions.md)
  (one new short section near the top).
- `CLAUDE.md`'s catalog auto-picks up the new file
  on `mdsmith fix CLAUDE.md`.

## Tasks

1. Rewrite the format string and drop
   `Feature.Verb()` plus the test that exercises
   it.
2. Update the bad fixtures' expected messages
   (twenty-one at the time this plan landed; the
   final count became eighteen after the
   convention rename removed three rule-level
   profile fixtures).
3. Write the
   [concepts doc](../docs/background/concepts/flavor-rule-convention-kind.md).
4. Add the link from the MDS034 README and the
   [conventions reference](../docs/reference/conventions.md).
5. Run `mdsmith fix CLAUDE.md PLAN.md` to refresh
   catalogs.
6. Run `go test ./...` and `go tool golangci-lint
   run`; both clean.

## Acceptance Criteria

- [x] MDS034 emits the new message format for every
      tracked feature; old wording does not appear
      in source or fixtures.
- [x] `Feature.Verb()` is removed and no caller
      references it.
- [x] All `bad/` fixtures pass with the new
      expected messages.
- [x] The
      [concepts doc](../docs/background/concepts/flavor-rule-convention-kind.md)
      exists and is reachable from the MDS034
      README and the
      [conventions reference](../docs/reference/conventions.md).
- [x] `mdsmith check .` reports no diagnostics
      across the repo.
- [x] `go test ./...` passes.
- [x] `go tool golangci-lint run` reports no
      issues.
