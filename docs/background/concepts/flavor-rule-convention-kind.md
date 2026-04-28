---
summary: >-
  How "flavor" (a property of the renderer), "rule"
  (a single check), "convention" (a project-wide
  bundle), and "kind" (a per-file role tag) differ
  in mdsmith, the cases where they overlap, and how
  the four concepts compose.
---
# Flavor, rule, convention, kind

mdsmith uses four related but distinct ideas to
constrain Markdown. A reader who is unsure which
mechanism they want — or whether they want more
than one — should read this page first.

## TL;DR

| Axis              | Flavor                             | Rule                         | Convention                         | Kind                              |
|-------------------|------------------------------------|------------------------------|------------------------------------|-----------------------------------|
| What it is        | A renderer's grammar               | A single lint check          | A project-wide bundle of rules     | A per-file role tag with rules    |
| Source of truth   | An external spec or implementation | An mdsmith rule package      | The codebase or `.mdsmith.yml`     | The user's `.mdsmith.yml`         |
| Question answered | Will the renderer interpret this?  | Does this match this rule?   | What kind of Markdown do we write? | What role does this file play?    |
| Scope             | Project-wide (one)                 | Per-feature                  | Project-wide (one)                 | Per-file (zero or many composed)  |
| Example           | `flavor: gfm` on MDS034            | MDS044 horizontal-rule-style | `convention: portable`             | `kinds: { plan: { rules: ... } }` |

## What each concept does

### Flavor

A flavor describes what a *renderer* does.
CommonMark won't auto-link a bare URL. GFM doesn't
render footnotes. Goldmark recognizes heading IDs
but the default GFM renderer does not. These are
facts about the parser/renderer, not opinions a
team can change. Flavor is read by MDS034 only.

### Rule

A rule describes one constraint. Most rules walk
the AST and emit diagnostics when a pattern
matches. Some rules also auto-fix. Each rule has
an ID, a name, a category, and a settings schema.
Rules exist independently; they don't know about
conventions or kinds.

### Convention

A convention is a named bundle that pairs a flavor
with a set of rule presets. Selecting a convention
applies both: MDS034 runs against the named flavor,
and the named rule presets are applied as a base
layer beneath the user's own rule config. Built-in
conventions: `portable`, `github`, `plain`. See
[conventions.md](../../reference/conventions.md).

### Kind

A kind is a named bundle of rule settings tagged
to specific files via front-matter `kinds:` or
glob `kind-assignment:`. Files of different kinds
get different settings. A document can carry zero
or many kinds; settings compose. See the
[file-kinds guide](../../guides/file-kinds.md).

## Why convention is not flavor

A flavor today corresponds to a renderer.
Promoting "portable" to a flavor would suggest
there is a renderer by that name, which there is
not. Flavor stays factual — does this parse and
render? Convention is opinionated — among forms
the renderer treats equally, which do we prefer?

Three concepts, three words: flavor (renderer),
convention (project), kind (file). Each word
telegraphs scope.

## Why convention is not kind

Convention and kind both bundle rule settings,
both deep-merge, and both surface as labeled
provenance layers. The difference is the
**selector**.

Convention selector says "this project, always."
One per project. Project-homogeneous.

Kind selector says "files matching front-matter
or glob." Many per project. File-heterogeneous.

A convention also implies a flavor. The renderer
is a project property; you cannot sensibly target
two renderers in one project. A kind doesn't, and
shouldn't, since the renderer is fixed by where
the project publishes.

The clearest framing:

- Convention answers "what kind of *Markdown* does
  this project write?" (one answer)
- Kind answers "what kind of *file* is this?"
  (zero or many answers)

## The fuzzy middle: features that parse but do not render

Some syntax parses on every flavor but is only
*recognized as a feature* on some. A GFM-style
table inside a CommonMark file parses fine. The
renderer emits a paragraph of literal pipes
instead of an HTML table. MDS034 still flags it
because the renderer does not render the syntax
as the named feature. The diagnostic uses the
wording "<flavor> does not interpret <feature>
as a feature" to make this explicit.

Bare URLs are the cleanest case where flavor and
rule overlap legitimately.

MDS034 with `flavor: commonmark` flags them
because CommonMark won't auto-link them. MDS012
(`no-bare-urls`) flags them on style grounds
regardless of flavor. Both diagnostics fire on the
same source for different reasons.

That overlap is intentional. A team using `flavor:
gfm` may still prefer explicit
`<https://example.com>` for readability. MDS012
enforces that even when the renderer would
auto-link.

## How the four compose

The merge order, oldest → newest:

1. `convention.<name>` — the convention preset
2. `default` — built-in defaults plus the user's
   top-level rules
3. `kinds.<name>` — each kind in the file's
   effective kind list
4. `overrides[i]` — each matching override entry

A team can write:

```yaml
convention: portable
kinds:
  plan:
    rules:
      max-file-length: { max-bytes: 50000 }
```

Plan-tagged files get portable's preset, then
default rules, then the plan-specific cap on top.
Deep-merge handles the rest. Convention gives the
floor; kinds adjust per file class.

## Practical guidance

Pick a flavor when the project targets a specific
renderer (GitHub, MkDocs, a static site generator,
plaintext readers). Pick rules when the team wants
to enforce one specific constraint. Pick a
convention when you want a curated bundle for the
whole project. Pick a kind when files of different
classes need different settings.

If you're seeing a diagnostic and aren't sure
which mechanism produced it, run [`mdsmith kinds
resolve <file>`](../../reference/cli.md). The
per-rule merge chain shows whether the value came
from a convention, default, kind, or override.
