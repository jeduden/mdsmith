---
id: 170
title: Audit link handling across mdsmith and the website
status: 🔲
summary: >-
  Survey every place mdsmith touches Markdown links — the
  linter (MDS027 and friends), the website rewriter, the
  Hugo render-link hook, and adjacent rule surfaces — and
  decide which gaps to close as concrete rules or
  pipeline fixes. PR #309 surfaced enough issues
  (subpath baseURL handling, `*ast.Image` blind spots,
  absolute targets, source-vs-rendered-URL depth) to
  justify a single sweep rather than another point fix.
model: opus
---
# Audit link handling across mdsmith and the website

## Goal

Catalog every link concern across mdsmith today. Rank
by user impact. Decide which ones become new rules,
rewriter changes, or scope decisions. The output is a
research doc at `docs/research/links/`. Follow-up plans
cite the doc instead of relitigating the trade-offs.

## Background

PR #309 (rule pages and synced-tree lint) surfaced
multiple link issues in close succession:

- MDS027 walks only `*ast.Link`, never `*ast.Image`, so
  broken image targets slip past the cross-file check.
- MDS027 short-circuits absolute paths
  (`/docs/rules/<id>/`), so the very URLs the website
  rewriter emits are not validated by the synced-tree
  lint.
- linkgraph's `ExtractLinks` intentionally skips
  reference-style links (`[label]: target`), so a
  broken ref-style def survives the check.
- The website rewriter hardcoded `/docs/rules/…` paths
  that ignored Hugo's configured `baseURL`; the
  render-link hook had to grow `relURL` support.
- Source-file relative paths resolve one directory
  level shallower than the rendered-URL would
  (`../reference/globs.md` from `docs/guides/file-kinds.md`
  becomes `/docs/guides/reference/globs/` if rendered
  literally instead of `/docs/reference/globs/`).
- `Markdown` titled links (`[x](url "title")`) are not
  matched by any rewriter regex; no source doc uses
  them today, but the gap is real.
- Issue [#47](https://github.com/jeduden/mdsmith/issues/47)
  proposes external-URL HTTP checking — a separate
  network-bound concern.

Several other concerns sit alongside these without a
home:

- Link style consistency inside a doc set
  (relative-vs-absolute, `.md`-vs-extensionless,
  inline-vs-reference). Mixed styles are common in
  mdsmith's own docs and reduce readability.
- Heading-anchor stability when renderers normalize
  differently (CommonMark vs GFM vs goldmark with
  attribute extensions).
- Wiki-style `[[target]]` links from Obsidian-flavored
  Markdown (see [plan
  168](168_obsidian-markdown-support.md)).
- Redirects / aliases for renamed pages.
- Cross-repo references (`@mdsmith/cli` → another
  mdsmith-managed repo).

A single research pass is cheaper than discovering each
of these mid-implementation in another PR.

## Tasks

1. Inventory every link surface in the codebase: list
   each rule that reads links (MDS012, MDS019, MDS021,
   MDS027, MDS035, MDS037, MDS049, MDS053, MDS054, and
   the directive-validating rules), the linkgraph
   package, the website rewriter regexes
   ([internal/release/website.go](../internal/release/website.go)),
   the Hugo render-link hook at
   [website/layouts/_default/_markup/](../website/layouts/_default/_markup/),
   and any CLI subcommand that surfaces link data
   ([mdsmith list backlinks](../docs/reference/cli/backlinks.md)).
   Note which node kinds each one walks
   (`*ast.Link`, `*ast.Image`, `*ast.AutoLink`,
   reference defs, autolinks-from-text) and which
   parts of the URL each one understands (scheme,
   absolute, relative, fragment, title).
2. Audit a sample corpus — `docs/`, `plan/`,
   `internal/rules/MDS*/README.md`, the synced
   `website/content/docs/` tree, and the README — for
   each link form actually in use. Record counts per
   form (inline vs ref, relative vs absolute, with vs
   without `.md` suffix, with vs without fragment,
   with vs without title) and per kind. This grounds
   later policy choices in real data.
3. Catalog every gap surfaced above plus any new ones
   the inventory turns up. For each gap, record:
   target rule / surface, observed example, the
   diagnostic mdsmith would emit today, the
   diagnostic it should emit, and the breakage on the
   published site if the gap stays open.
4. For each gap, decide one of: (a) new rule with an
   MDS id, (b) extension to an existing rule, (c)
   build-time rewriter change, (d) Hugo render-time
   change, or (e) deliberately not addressed (scope
   decision). Capture the reasoning in one paragraph
   each.
5. Sketch a minimal API for a shared "link policy"
   config block under `.mdsmith.yml` — at minimum:
   preferred relative-vs-absolute style per kind,
   preferred extension policy, allow-list of
   external-URL skip patterns, image-target
   validation toggle. This is design only; the
   implementation cost lands in the follow-up plans.
6. Decide whether MDS027 should grow image-target
   validation, absolute-path resolution against a
   configured site root, and ref-style coverage —
   versus splitting those into new rules. Look at the
   gomarklint precedent
   ([docs/research/mdbase-vs-mdsmith/](../docs/research/mdbase-vs-mdsmith/))
   for prior art.
7. Decide whether `index.md` / `_index.md` /
   trailing-slash conventions belong in a rewriter
   layer, a render-link layer, or both (the current
   PR #309 design splits them; the audit confirms or
   reverses that split).
8. Decide whether issue #47 (external URL HTTP
   probing) lands as part of the same rule family or
   as a standalone opt-in rule. Note the trade-off
   with offline CI runs.
9. Write the research doc at
   `docs/research/links/README.md` (front matter, a
   `summary:`, sections for each gap and the
   decision). Link it from the docs catalog so
   `mdsmith fix` picks it up.
10. File one follow-up plan per "act on this" decision
    from step 4. Each follow-up cites this audit.
    Defer implementation to those plans.

## Acceptance Criteria

- [ ] `docs/research/links/README.md` exists, passes
      `mdsmith check`, and is in the homepage catalog.
- [ ] Every gap surfaced by PR #309 review is
      catalogued with a target surface and a decision.
- [ ] Inventory of link rules and link-aware code is
      complete (one entry per file).
- [ ] Corpus audit lists per-form counts for inline
      vs ref-style, relative vs absolute, with vs
      without `.md`, with vs without fragment.
- [ ] A shared `links:` config sketch is in the doc
      (design only — no parser changes here).
- [ ] At least one follow-up plan file is filed per
      "act on this" decision.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no issues.
