---
summary: >-
  Audit of every link surface in mdsmith — the link-aware
  rules, the linkgraph package, the website rewriter, and
  the Hugo render-link hook — with a corpus census, a
  catalogued gap list, a decision per gap, and a sketched
  shared `links:` config block. Follow-up plans 171–173
  cite this doc instead of relitigating the trade-offs.
---
# Link handling audit

This note surveys how mdsmith reads, validates, and
rewrites Markdown links across the linter, the
[`linkgraph`](../../../internal/linkgraph/linkgraph.go)
package, the
[website rewriter](../../../internal/release/website.go),
and the
[Hugo render-link hook](../../../website/layouts/_default/_markup/render-link.html).
It grounds policy choices in a corpus census, catalogues
every gap PR #309 surfaced plus the inventory's new ones,
and records one decision per gap. Implementation is
deferred to the follow-up plans named in each decision.

## Inventory of link surfaces

One entry per file. "Nodes" lists the goldmark AST node
kinds the surface walks for links; "URL parts" lists
which components of the target it inspects.

| Surface                                                                                      | Nodes                                                                      | URL parts                                                                 | What it does                                                                                                                       |
|----------------------------------------------------------------------------------------------|----------------------------------------------------------------------------|---------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------|
| MDS012 no-bare-urls (`internal/rules/nobareurls/rule.go`)                                    | `*ast.Text`                                                                | scheme (http/https)                                                       | Flags raw `https?://` in text not already inside a link, autolink, or code; wraps in `<>` on fix.                                  |
| MDS019 catalog (`internal/rules/catalog/rule.go`)                                            | `<?catalog?>` PI                                                           | none (front-matter fields)                                                | Generates an index list; warns when a templated field value contains a `](` link-injection sequence.                               |
| MDS021 include (`internal/rules/include/rule.go`)                                            | `<?include?>` PI                                                           | relative file path only                                                   | Resolves the `file:` parameter; rejects absolute paths and `..` escapes; detects cycles.                                           |
| MDS027 cross-file-reference-integrity (`internal/rules/crossfilereferenceintegrity/rule.go`) | `*ast.Link` with `Reference == nil` only (via `linkgraph.ExtractLinks`)    | relative path, fragment; rejects scheme/host; **short-circuits absolute** | Checks inline link targets exist on disk and that `#anchor` matches a heading.                                                     |
| MDS032 no-empty-alt-text (`internal/rules/noemptyalttext/rule.go`)                           | `*ast.Image`                                                               | none (alt text only)                                                      | Flags images with empty/whitespace alt text. The **only** rule that walks `*ast.Image`.                                            |
| MDS035 toc-directive (`internal/rules/tocdirective/rule.go`)                                 | `*ast.Paragraph` text                                                      | none                                                                      | Flags renderer-specific TOC markers (`[TOC]`, `[[toc]]`, `${toc}`).                                                                |
| MDS037 duplicated-content (`internal/rules/duplicatedcontent/rule.go`)                       | `*ast.Paragraph`                                                           | none                                                                      | Cross-file duplicate-paragraph fingerprinting; skips generated bodies.                                                             |
| MDS038 toc (`internal/rules/toc/rule.go`)                                                    | headings via `mdtext.CollectTOCItems`                                      | fragment only                                                             | Generates the `<?toc?>` body with GitHub-style `#anchor` links.                                                                    |
| MDS043 no-reference-style (`internal/rules/noreferencestyle/rule.go`)                        | `*ast.Link`/`*ast.Image` with `Reference != nil`; regex for defs/footnotes | none                                                                      | Flags reference-style links/images and footnote refs.                                                                              |
| MDS049 no-space-in-link-text (`internal/rules/nospaceinlinktext/rule.go`)                    | `*ast.Link`, `*ast.Image`                                                  | none (bracket whitespace)                                                 | Flags leading/trailing space inside `[...]`.                                                                                       |
| MDS053 no-unused-link-definitions (`internal/rules/nounusedlinkdefinitions/rule.go`)         | `*ast.Link`/`*ast.Image` with `Reference != nil`; regex for defs           | none                                                                      | Flags unused/duplicate `[label]:` definitions.                                                                                     |
| MDS054 no-undefined-reference-labels (`internal/rules/noundefinedreferencelabels/rule.go`)   | regex on source (no AST link walk)                                         | none                                                                      | Flags reference links/images whose label has no definition.                                                                        |
| `linkgraph.ExtractLinks` (`internal/linkgraph/linkgraph.go`)                                 | `*ast.Link` with `Reference == nil` only                                   | path, fragment; rejects scheme/host/`//`                                  | The shared inline-link extractor. **Skips images, autolinks, and reference-style links by design.**                                |
| `linkgraph.ExtractRefLinks` (`internal/linkgraph/refs.go`)                                   | `*ast.Link` with `Reference != nil`                                        | normalized label only                                                     | Complements `ExtractLinks` for ref-style links; **not consumed by MDS027 today**.                                                  |
| `astutil` (`internal/rules/astutil/astutil.go`)                                              | block nodes only                                                           | none                                                                      | Heading/paragraph text helpers; no link walking.                                                                                   |
| Website rewriter (`internal/release/website.go`)                                             | inline `](...)` and `[label]: ...` defs via ~9 regexes                     | path, fragment; **not titles, not images**                                | Sync-time rewrite of repo-relative `.md` paths to site-absolute or GitHub URLs; guarded by `applyOutsideCode`.                     |
| Hugo render-link hook (`website/layouts/_default/_markup/render-link.html`)                  | all rendered links                                                         | path, fragment, site prefix                                               | Resolves source-relative `.md` via `GetPage`, strips `index.md`/`_index.md`, reattaches fragment, applies non-root baseURL prefix. |
| `mdsmith list backlinks` (`cmd/mdsmith/backlinks.go`)                                        | `*ast.Link` via `ExtractLinks`                                             | resolved path                                                             | Lists workspace links pointing at a file; inherits all `ExtractLinks` blind spots.                                                 |

The recurring theme: `*ast.Link` (inline only) is the
well-trodden path. `*ast.Image`, `*ast.AutoLink`,
reference-style links, absolute paths, and link titles
are each understood by at most one isolated surface, and
the shared `linkgraph.ExtractLinks` — the spine of
MDS027 and backlinks — sees none of them.

## Corpus census

Counts over `docs/` (72 files, research/security excluded
from this census), `plan/` (82), `internal/rules/MDS*/`
READMEs (60), and root `README.md`. Approximate, derived
from ripgrep over each corpus; the point is the
distribution, not the exact integer.

| Metric                    | Docs | Plan | Rules | README |
|---------------------------|------|------|-------|--------|
| Inline links              | 274  | 330  | 159   | 42     |
| Reference-style uses      | 146  | 99   | 14    | 3      |
| Reference definitions     | 147  | 51   | 14    | 6      |
| Relative `../` targets    | 140  | 271  | 103   | 0      |
| Relative same-dir targets | 59   | 23   | 0     | 0      |
| Absolute `/root` targets  | 2    | 0    | 0     | 0      |
| External http(s)          | 16   | 8    | 6     | 1      |
| Fragment-only (`#x`)      | 3    | 9    | 11    | 0      |
| `.md#anchor` targets      | 12   | 3    | 3     | 0      |
| Links with a title        | 0    | 1    | 0     | 0      |
| Images                    | 0    | 0    | 1     | 1      |
| Empty alt text            | 0    | 0    | 1     | 0      |
| Autolinks `<...>`         | 4    | 3    | 1     | 0      |
| Mailto                    | 0    | 0    | 0     | 0      |

Reading the data:

- **Style is split, not consistent.** `docs/` is ~50%
  reference-style; `plan/` and rule READMEs are
  overwhelmingly inline. There is no project-wide
  relative-vs-absolute or inline-vs-reference
  convention, and the two styles are mixed within the
  same doc set.
- **Internal links are relative.** Absolute-from-root
  targets are nearly nonexistent in source (2 in all of
  `docs/`); the site-absolute form only appears *after*
  the rewriter runs — exactly the form MDS027
  short-circuits.
- **Extensionless internal links exist.** 59 same-dir
  `foo.md`-style targets in `docs/` versus 0 in rule
  READMEs (which are pure `../`), so an extension policy
  would not be a no-op.
- **Titles and images are effectively unused in
  published source** (0 titled links in `docs/`, 0
  images in `docs/`/`plan/`). The gaps are real but the
  blast radius today is small.

## Gap catalogue and decisions

Each gap: the surface, an observed example, what mdsmith
emits today, what it should emit, the site breakage if
left open, and the decision — (a) new rule, (b) extend an
existing rule, (c) rewriter change, (d) render-time
change, or (e) deliberately not addressed.

### G1 — MDS027 ignores `*ast.Image`

`ExtractLinks` walks only `*ast.Link`, so
`![diagram](missing.png)` is never resolved. Today:
silent pass. Should: same broken-target diagnostic as a
text link. Site breakage: a 404 image ships to
mdsmith.dev with no lint signal. **Decision: (b)** —
extend MDS027 via a `validate-images` toggle (default
on). Images and links share resolution logic; a separate
rule would duplicate `ParseTarget`. Plan 171.

### G2 — MDS027 short-circuits absolute paths

`[x](/rules/MDS027/)` returns early because
`ParseTarget` rejects a non-empty path that looks
rooted. Today: silent pass. Should: resolve against a
configured site root and verify the page exists.
Breakage: the rewriter's own output (site-absolute) is
the one form the synced-tree lint cannot validate — a
broken rewrite is invisible until a human clicks the
link. **Decision: (b)** — extend MDS027 with an optional
`site-root:` so absolute targets resolve to a workspace
path. Off when unset, preserving today's behavior. Plan
171.

### G3 — linkgraph skips reference-style links

`ExtractLinks` filters `Reference == nil`, so a broken
`[a]` whose `[a]: ./gone.md` def points nowhere
survives. `ExtractRefLinks` exists but nothing wires it
into MDS027. Today: silent pass. Should: resolve
ref-style targets like inline ones. Breakage: a broken
ref-def ships. **Decision: (b)** — feed
`ExtractRefLinks` results through the same resolver in
MDS027. Plan 171.

### G4 — Rewriter ignored Hugo baseURL

PR #309's first cut hardcoded `/docs/rules/…`, breaking
a non-root Pages deploy. **Decision: (e), already
resolved.** The render-link hook now derives the prefix
from `site.Home.RelPermalink`. Recorded here so a future
reader does not refile it; Plan 171 adds a regression
test asserting the prefix under a subpath baseURL.

### G5 — Source-vs-rendered URL depth

`../reference/globs.md` from `docs/guides/file-kinds.md`
would resolve one level too shallow if emitted
literally. **Decision: (e), already resolved.** The
render-link hook calls Hugo `GetPage`, which returns the
target's absolute permalink and absorbs the rendering
depth. The audit confirms the PR #309 split (see G7).

### G6 — Titled links unmatched by rewriter

`[x](../y.md "title")` is not matched by any rewriter
regex (`\S+` stops at the space before the title), so it
would ship as a dead repo-relative path. Corpus: 0 in
`docs/`, 1 in `plan/` (unpublished). Today: passes the
rewriter unchanged. Should: rewrite path, preserve
title. **Decision: (c)**, low priority — widen the
rewriter regexes to capture an optional
`(\s+"[^"]*")?` tail and re-emit it. Plan 173.

### G7 — index.md / _index.md / trailing slash split

PR #309 splits this: the **rewriter** drops a trailing
`index.md` to a directory path and keeps synced files
filesystem-valid for MDS027; the **render-link hook**
strips `index.md`/`_index.md` and normalizes the
trailing slash at render time using Hugo's content tree.
**Decision: (e), confirm the split.** It is correct:
basename-to-slug transforms depend on Hugo's tree, which
the regex rewriter cannot see, while MDS027 needs the
on-disk filename intact. Reversing it would either break
the synced-tree lint or duplicate Hugo's routing in Go.
No change; documented so the next PR does not re-derive
it.

### G8 — No link-style consistency rule

Mixed relative/absolute, `.md`/extensionless, and
inline/reference within one doc set (census: `docs/` is
~50% reference-style, rule READMEs ~0%). Today: no
signal. Should: an opt-in rule flags deviation from a
per-kind declared style. **Decision: (a)** — new opt-in
rule `link-style` reading the shared `links:` block
(below). Plan 172.

### G9 — Heading-anchor stability across renderers

CommonMark, GFM, and goldmark-with-attributes normalize
heading slugs differently, so an anchor valid under one
renderer 404s under another. **Decision: (e), not
addressed.** mdsmith already commits to goldmark as the
single anchor authority (MDS038 generates GitHub-style
slugs and MDS027 checks against the same computation).
Multi-renderer anchor portability has no concrete user
and would fork the anchor model. Revisit only if a
convention targets a non-goldmark flavor.

### G10 — Wiki-style `[[target]]` links

Not parsed by goldmark's default extensions; invisible
to every surface. **Decision: (e), deferred to
[plan 168](../../../plan/168_obsidian-markdown-support.md).**
Wikilink validation is in scope for the Obsidian
convention there (extended MDS027 settings); duplicating
it here would pre-empt that design.

### G11 — Redirects / aliases for renamed pages

A renamed page breaks inbound links with no mdsmith
signal. **Decision: (e), out of linter scope.** Hugo
`aliases` front matter handles published redirects;
in-repo, this is a rename-refactor concern (LSP rename
already rewrites workspace links). No rule; recorded so
it is not refiled as a lint gap.

### G12 — Cross-repo references

`@mdsmith/cli` → another mdsmith-managed repo cannot be
resolved on the local filesystem. **Decision: (e),
deferred, no concrete need.** No current doc links
cross-repo by package name; revisit when a multi-repo
workspace exists. Listed for completeness.

### G13 — Issue #47 external URL HTTP probing

[Issue #47](https://github.com/jeduden/mdsmith/issues/47)
proposes HTTP-checking external URLs. **Decision: (a),
standalone opt-in rule, not the MDS027 family.** MDS027
is a pure, offline, deterministic filesystem check;
folding network I/O into it would make every CI run
network-bound and flaky. A separate off-by-default rule
(`external-link-check`) with skip patterns, caching, and
a timeout keeps offline CI fast and the failure modes
isolated. Tracked by issue #47; Plan 172 carries the
shared skip-pattern config it depends on, so the rule
lands on a config foundation rather than inventing its
own.

## Sketched `links:` config block

Design only — no parser changes in this audit. A shared
block under `.mdsmith.yml`, deep-merged per kind like
every other rule config, consumed by MDS027 (G1–G3) and
the new `link-style` rule (G8), and supplying the skip
list issue #47's rule (G13) reuses:

```yaml
links:
  # G2: resolve absolute targets against this workspace
  # path; unset preserves today's short-circuit.
  site-root: ""

  # G1: validate *ast.Image targets like text links.
  validate-images: true

  # G3: resolve reference-style targets too.
  validate-reference-style: true

  # G8: preferred styles; flagged only by the opt-in
  # link-style rule, overridable per kind.
  style:
    path: relative        # relative | absolute
    extension: keep        # keep | strip   (.md suffix)
    form: any              # any | inline | reference

  # G13: regexes for external URLs the (separate,
  # opt-in) HTTP-check rule must skip.
  external-skip:
    - '^https?://localhost'
```

Per-kind override example: rule READMEs are pure `../`
inline, so a `rule-readme` kind could pin
`style.form: inline`; `docs` could stay `form: any`
until the corpus is migrated.

## MDS027: extend vs split

**Decision: extend MDS027** for G1 (images), G2
(absolute-against-site-root), and G3 (ref-style),
gated by the `links:` toggles. Rationale: all three are
the same operation — "does this target resolve?" — over
a wider node set and a configurable root. The gomarklint
precedent in
[learn-from-mdbase](../mdbase-vs-mdsmith/learn-from-mdbase.md)
keeps file-integrity unified rather than fragmenting it
per node kind; splitting would duplicate `ParseTarget`
and the heading-set build across three rules and split
one diagnostic class across three IDs. Network probing
(G13) is the one piece that does **not** belong in
MDS027 — it is impure and offline-hostile — so it splits
out as its own opt-in rule.

## Follow-up plans

Each cites this audit; implementation is deferred to
them.

- [Plan 171](../../../plan/171_mds027-link-integrity-hardening.md)
  — MDS027 link-integrity hardening: G1, G2, G3, plus
  the G4 subpath regression test.
- [Plan 172](../../../plan/172_link-style-rule-and-config.md)
  — link-style rule and shared `links:` config: G8 and
  the `links:` parser, including the `external-skip`
  list G13's rule consumes.
- [Plan 173](../../../plan/173_rewriter-titled-links.md)
  — rewriter tolerates titled links: G6.

G7, G9, G11, G12 are scope decisions (no plan). G10 is
deferred to
[plan 168](../../../plan/168_obsidian-markdown-support.md).
G13 is tracked by
[issue #47](https://github.com/jeduden/mdsmith/issues/47)
and depends on Plan 172's config.
