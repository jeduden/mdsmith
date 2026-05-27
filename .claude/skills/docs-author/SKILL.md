---
name: docs-author
description: >-
  Write Markdown documentation that picks the right
  Diátaxis type (tutorial, how-to, reference,
  background), derives matching title and summary
  fragments, and passes a global-English and
  anti-slop polish. Trigger when the user asks to
  "draft a doc", "write a how-to", "draft a
  reference", "write a background page", "rewrite
  this in the house voice", "summary for this
  page", "title for this page", "polish for
  non-native readers", "this reads like AI",
  "remove AI tells / slop", or "what type of doc
  is this". Skip this skill for content-rule fixes
  (line length, heading hygiene, readability) —
  those are enforced by `mdsmith check`.
user-invocable: true
argument-hint: "[draft | revise | fragments]"
allowed-tools: >-
  Bash(mdsmith:*),
  Bash(go run ./cmd/mdsmith:*),
  Bash(ls:*),
  Bash(find:*),
  Bash(grep:*),
  Bash(git ls-files:*)
---

mdsmith's docs live in four homes that match the
[Diátaxis](https://diataxis.fr/) compass:
`docs/guides/` (how-to and tutorial),
`docs/reference/` (reference), and
`docs/background/` (explanation). Each type has a
different audience in a different mode — and a
different voice that follows from that. This skill
picks the type, writes to its rules, and emits the
fragments the catalog and LSP already consume.

Two filters run on every draft on top of the
type-specific rules:

- **Global English** — short sentences, active
  voice, controlled vocabulary, no idioms — so a
  non-native reader can parse the page on a first
  pass.
- **Anti-slop** — block the vocabulary, phrasing,
  and structure patterns that read as
  LLM-generated. The catalog lives in
  `slop-patterns.md` (sibling file).

## When to run this skill

- Drafting a new doc page from a brief or a stub.
- Revising an existing page that reads as
  promotional, padded, or AI-shaped.
- Generating fragments — `title`, `summary`, link
  text, hover blurb — for a page that lacks them
  or whose fragments do not match the page type.
- Auditing a page whose type is unclear (a
  reference that started explaining, a how-to that
  drifted into background).

Skip this skill for line-length, heading-style,
table alignment, or other surface fixes — run
`mdsmith fix` for those.

## Modes

Pass the mode as the skill argument.

- **`draft`**. Take a brief plus an optional
  target path, classify the type via the compass,
  and write a new file under the matching
  `docs/<area>/` subtree.
- **`revise`** (default). Take an existing file,
  re-classify it, rewrite to the type's voice and
  shape, then run the global-English and
  anti-slop passes.
- **`fragments`**. Read a file's body and emit
  `title:` and `summary:` front matter plus
  candidate link text and hover blurb. No body
  edits.

Default to `revise` when no argument is given.

## The compass — pick one of four types

Apply two questions to every page. The answers
fix the type and the voice.

|                            | **Action** (doing) | **Cognition** (thinking) |
| -------------------------- | ------------------ | ------------------------ |
| **Acquisition** (studying) | tutorial           | background               |
| **Application** (working)  | how-to             | reference                |

- *Action or cognition?* — is the reader trying
  to **do** a thing, or trying to **understand** a
  thing?
- *Acquisition or application?* — is the reader
  **studying** (no prior competence assumed) or
  **at work** (already competent, has a goal)?

If the page is mixing two quadrants, split it.
One page, one type.

### Tutorial — learning-oriented (acquisition × action)

- **Audience mode**: a learner with no prior
  encounter. The author drives.
- **Voice**: narrative, second person, present
  tense. Concrete steps with a single, verifiable
  outcome at the end.
- **Title pattern**: activity-framed —
  `Get started with mdsmith`, `Your first mdsmith
  rule`. Never a noun phrase.
- **Shape**: prerequisites → numbered steps →
  outcome → next steps.
- **Banned**: explanation of why; API
  exhaustiveness; alternative paths.
- **Home**: `docs/tutorials/`.

### How-to guide — task-oriented (application × action)

- **Audience mode**: a competent practitioner who
  arrived with a specific goal.
- **Voice**: imperative. Goal-named. Minimal
  scaffolding.
- **Title pattern**: bare-infinitive verb phrase —
  `Install mdsmith on macOS`, `Migrate from
  markdownlint`.
- **Shape**: one goal → ordered steps → a verified
  end state.
- **Banned**: teaching the reader; reference
  tables; explanations of design.
- **Home**: `docs/guides/`.

### Reference — information-oriented (application × cognition)

- **Audience mode**: a working practitioner who
  needs one fact, fast. Will scan, not read.
- **Voice**: neutral, declarative, exhaustive. No
  narrative. No opinions. No "you".
- **Title pattern**: canonical noun phrase or
  identifier — `` `mdsmith check` ``, `Glob
  patterns`, `Section schema`.
- **Shape**: identifier → one-line definition →
  parameters/fields table → exit codes / errors →
  examples → see also.
- **Banned**: rhetorical framing; tutorial steps;
  history; rationale.
- **Home**: `docs/reference/`.

### Background — understanding-oriented (acquisition × cognition)

- **Audience mode**: a reader at study, building
  a mental model. Willing to follow prose.
- **Voice**: discursive, may compare alternatives
  and discuss trade-offs. Still concrete.
- **Title pattern**: noun phrase explaining the
  thing — `How generated sections work`,
  `Placeholder grammar`.
- **Shape**: framing → concept → how it relates
  to other concepts → when it matters → links to
  reference and how-to.
- **Banned**: ordered steps; CLI flag tables;
  goal-naming.
- **Home**: `docs/background/`.

## Workflow

### draft

1. Read the brief. If the type is not stated,
   apply the compass and name the type back to
   the user.
2. Pick the target path under
   `docs/<area>/<slug>.md`. The slug must satisfy
   the kind's `path-pattern:` if one is declared.
3. Write the front matter first — `title`,
   `summary` — using the per-type formulas in
   `## Fragment formulas` below.
4. Write the body to the type's shape. Lean on
   existing pages in the same area as voice
   templates.
5. Run the global-English pass, then the
   anti-slop pass (see below).
6. Run `mdsmith check <path>` and fix anything it
   surfaces.

### revise

1. Read the file. Classify the type via the
   compass.
2. If the file mixes types, ask the user before
   continuing. Splitting is a structural change,
   not a copy edit.
3. Rewrite to the type's voice and shape.
   Preserve the file's facts; replace the
   prose around them.
4. Refresh fragments (`title`, `summary`) if the
   re-classification changed the type.
5. Run the global-English pass, then the
   anti-slop pass.
6. Run `mdsmith check <path>`.

### fragments

1. Read the file. Classify the type.
2. Derive `title:` and `summary:` from the body
   using the per-type formula.
3. Print three link-text candidates and one
   hover blurb (≤120 chars).
4. Patch the front matter if the user accepts.

## Fragment formulas

A page's `summary` is read three places: the
catalog row that lists it, the LSP hover that
previews it, and the search snippet that
surfaces it. All three modes are scanning, not
reading. Same formula for all three.

The formulas below are per-type because the
fragment carries the page's promise — and that
promise is type-specific.

### `summary` — one sentence, ≤ 25 words, ≤ 160 chars

- **Tutorial**: state the outcome — "Build your
  first mdsmith rule end to end, from failing
  test to green check."
- **How-to**: name the goal — "Install mdsmith
  on macOS, Linux, and Windows via npm, mise, or
  the GitHub release."
- **Reference**: name the surface and its
  contract — "CLI commands, flags, exit codes,
  and output format."
- **Background**: name the concept and what it
  resolves — "How generated sections work —
  markers, directives, and fix behavior."

Rules that apply to every type:

- Declarative. Not "this page covers…", not "in
  this guide we will…".
- One sentence. If two ideas need joining, the
  page is two pages.
- Concrete subject. No floating "it".
- Active voice.
- No marketing adjectives (powerful, seamless,
  comprehensive, robust, …).

### `title` / H1

The H1 and the `title` front matter agree.

- **Tutorial**: activity-framed verb phrase.
- **How-to**: bare-infinitive verb phrase.
- **Reference**: canonical noun phrase or
  identifier; code-fenced if it is one.
- **Background**: noun phrase explaining the
  thing.

### Link text and hover

- **Link text**: never "click here", never the
  raw filename. Verb-led for how-to/tutorial;
  noun-led for reference/background.
- **Hover blurb**: same content as `summary`,
  capped at 120 chars for editor surfaces.

## Global-English pass

Run on every draft after the type-specific shape
is in place. The rules come from Simplified
Technical English ([ASD-STE100][ste]) and the
Plain Language standard
([ISO 24495-1:2023][iso24495]); they are tuned so
a non-native reader can parse on a first read.

[ste]: https://en.wikipedia.org/wiki/Simplified_Technical_English
[iso24495]: https://www.iso.org/standard/78907.html

- **One concept, one word.** Pick one of
  *configure / set up / set* per page and stick.
  No synonym shuffle.
- **Sentence cap.** ≤ 20 words in procedural
  text (how-to, tutorial, reference). ≤ 25 in
  background. Split when over.
- **Active voice by default.** Passive only when
  the agent is unknown or irrelevant.
- **Concrete subject.** No floating *it* or
  *this*; name the thing.
- **No idioms, no phrasal-verb-where-a-verb-works.**
  *Set up → configure. Find out → learn. Figure
  out → determine. Look into → investigate.*
- **Define jargon at first use** or link to a
  reference page that does.
- **Positive constructions.** *Disconnect before
  X* beats *do not stay connected during X*.
- **Parallel structure.** Lists and headings
  share a grammatical shape — all verb phrases,
  or all noun phrases, never mixed.
- **One sentence per line in source.** mdsmith
  wraps; the source line break does not.

## Anti-slop pass

Run after the global-English pass. The catalog
lives in `slop-patterns.md`. Load it, then walk
the draft against each category. The catalog
covers:

- **Vocabulary tells** — overused LLM words.
- **Phrasal tells** — set phrases.
- **Sentence openers** — banned at sentence
  start.
- **Structural tells** — rule-of-three garnish,
  the hedging seesaw, meta-narration.
- **Tone tells** — promotional, obsequious,
  padded.
- **Formatting tells** — bulleted lists with
  bolded label-and-colon openers used as the
  default rhythm; em-dash overuse.

Rewriting rules:

- **Catch the pattern, not the word.** Replacing
  *delve* with *explore* yields different slop.
  Recast the sentence so the word is not needed.
- **Plain replaces clever.** Drop padded
  qualifiers (*essentially, fundamentally,
  ultimately*) without substitution.
- **Cut meta-commentary.** "This document
  explains…" → delete and start with the actual
  content.
- **Per-type allowances** — bulleted lists with
  bolded openers are *allowed in reference*
  (parameter tables in list form), *flagged
  elsewhere* when used as default rhythm. The
  rule of three is *allowed* when the three
  items are genuinely the contents (e.g.
  `check`, `fix`, `lsp`).

## Tensions to know up front

- **Em dashes.** Allowed; capped. ≤ 1 per
  paragraph, ≤ 2 per page.
- **Hedging language** (*may, in some cases*).
  Allowed in background; restricted in how-to
  and reference where the answer must be
  definite.
- **Bulleted lists with bolded labels.** Allowed
  in reference, flagged elsewhere as default
  rhythm.

## Project conventions to respect

- **Front matter.** Most pages carry `title` and
  `summary`. CLI reference pages use `command` and
  `summary` (per the `cli-command` kind schema).
  Plan files also carry `status`. Do not invent
  new keys without a schema entry.
- **Voice from CLAUDE.md.** Descriptions name
  *what data must satisfy what condition*. Name
  the inputs (front matter fields, glob, heading
  level) — not just the mechanism. Avoid vague
  verbs (*match, sync, reflect*) without saying
  what is checked against what.
- **Linter configuration.** Do not edit
  `.mdsmith.yml` to make a draft pass. Rewrite
  the draft.
- **Generated sections.** Do not edit content
  between `<?…?>` and `<?/…?>` markers. Edit the
  directive parameters or the source file, then
  `mdsmith fix`.

## Notes

- This skill writes prose; `markdown-audit`
  audits structure (kinds, schemas, catalogs).
  They compose: run `markdown-audit` first if the
  page's home or kind is wrong, then this skill
  for the writing.
- The compass is a course-correction tool, not a
  cage. Some pages legitimately straddle types —
  a how-to with a one-paragraph background
  preface, a reference with a worked example.
  The test is: does the audience-mode stay the
  same throughout? If not, split.
- For every push-back from the user ("that's
  intentional"), prefer leaving the prose alone
  over adding a per-page exception. The skill is
  advisory.
