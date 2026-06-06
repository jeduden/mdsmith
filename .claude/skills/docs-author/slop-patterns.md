---
title: Slop patterns
summary: >-
  Vocabulary, phrasing, structural, tone, and
  formatting patterns the `docs-author` anti-slop
  pass blocks. Each entry is a pattern the skill
  rewrites at the sentence or paragraph level,
  not a word the skill swaps in place.
---
# Slop patterns

Patterns that mark prose as LLM-generated.

The list is curated from
[Wikipedia: Signs of AI writing][wp-signs] and
the community anti-slop skills (`unslop`,
`the-antislop`, `avoid-ai-writing`).

Walk every draft against each section. Fix a hit
by recasting the sentence — swapping the word
leaves the same shape behind, and that shape is
the tell.

The mechanical sections — "Vocabulary tells",
"Phrasal tells", and "Sentence openers" — also
ship as the built-in [`no-llm-tells`
convention][conv], which MDS056 and MDS055 enforce
in CI and the editor without a model in the loop. A
drift-checker test keeps the convention's lists a
subset of this catalog. The structural, tone, and
formatting sections below need contextual judgement
and stay skill-only.

[wp-signs]: https://en.wikipedia.org/wiki/Wikipedia:Signs_of_AI_writing
[conv]: ../../../docs/reference/conventions.md

## Vocabulary tells

Words that flag as LLM output. Avoid in any
register; if one is the only right word for a
fact, leave it — context decides.

- delve, dive into, dive deep, deep dive
- tapestry, landscape (figurative), realm
- testament (to), stands as a testament
- vibrant, pivotal, robust, seamless
- leverage (as a verb), unlock, unleash, harness
- embark, journey (figurative), navigate
  (figurative)
- foster, showcase, emphasize, enhance,
  highlight, align with
- crucial, essential, comprehensive, holistic
- multifaceted, nuanced, intricate
- paradigm, ecosystem, transformative
- vital, profound, paramount

Fix recipe: recast so the word is not the bearer
of the claim. *"X plays a pivotal role"* → *"X
does Y"*.

## Phrasal tells

Set phrases that flag the source.

- "it's important to note that"
- "it's worth mentioning that"
- "in today's fast-paced world"
- "in the digital age"
- "in the realm of"
- "in the world of"
- "at its core"
- "plays a crucial role"
- "stands as a testament to"
- "a deep dive into"
- "ever-evolving X"
- "as we navigate"
- "harness the power of"
- "unlock the potential of"
- "embark on a journey"
- "by understanding X, you can Y"
- "when it comes to X"
- "navigating the complexities of"

Fix recipe: delete the framing; lead with the
content the framing was hiding.

## Sentence openers

Banned as the first word of a sentence. Each
shifts the sentence's job from claim to
commentary.

- Certainly,
- Moreover,
- Additionally,
- Furthermore,
- Indeed,
- Notably,
- Importantly,
- Crucially,
- Essentially,
- Ultimately,
- Fundamentally,
- Basically,
- In essence,
- In conclusion,
- To summarize,
- To sum up,

Fix recipe: drop the opener. If the sentence
needs it to make sense, the sentence is doing
two jobs — split it.

## Structural tells

Patterns of arrangement, not vocabulary.

- **Rule-of-three garnish.** A trio of adjectives
  or noun phrases used as decoration: *"clear,
  concise, and compelling"*, *"fast, reliable,
  and scalable"*. Allowed when the three items
  are genuinely the contents (e.g. *`check`,
  `fix`, `lsp`*); banned when they are filler.
- **Hedging seesaw.** *"X isn't just Y — it's
  Z"*, *"not just X but also Y"*, *"this isn't a
  bug, it's a feature"*. Banned in any register.
- **Meta-narration.** *"This document covers…"*,
  *"In this guide we will…"*, *"Let's explore
  how…"*. Banned. Start with the content.
- **Uniform sentence length.** Every sentence
  12–18 words. Read aloud; if rhythm is flat,
  break the pattern — one short sentence breaks
  it.
- **Default scaffold.** *"First, …. Second, ….
  Finally, …"* as the page's default skeleton.
  Use a list instead.
- **Recap block at the end.** *"In summary,
  …"*, *"To recap, …"*, *"In conclusion,
  …"*. Banned. The page either earned its end
  or it did not.
- **Bilateral framing.** *"On one hand X, on the
  other hand Y"* as default rhythm. Use only
  when both hands are genuinely the contents.

## Tone tells

- **Promotional / press-release.** *"powerful,
  cutting-edge, industry-leading"*. Reference
  pages especially must stay neutral.
- **Obsequious.** *"Great question!"*,
  *"Certainly!"*, *"I'd be happy to…"*. Banned.
- **Padded qualifiers.** *essentially,
  fundamentally, ultimately, basically, really,
  simply, just*. Delete without substitution
  unless the qualifier carries a real claim.
- **Vague intensifiers.** *very, extremely,
  incredibly, truly, quite*. Cut or replace
  with a measurable claim.
- **Promotional adjectives in `summary`.**
  *powerful, seamless, comprehensive, robust,
  effortless*. Banned in front matter — the
  catalog row carries the page's promise and
  must be factual.

## Formatting tells

- **Bulleted list of `**Label**: prose` items as
  default rhythm.** Allowed in reference
  (parameter tables in list form); flagged in
  how-to, tutorial, and background when used as
  the page's main shape.
- **Em-dash overuse.** Cap at 1 per paragraph,
  2 per page. Replace runs of em-dashes with
  shorter sentences or parentheses.
- **Emoji as bullet.** Banned. mdsmith front
  matter uses `status: ✅` as data, not as
  decoration; emoji never lead a list item.
- **Code fences for non-code.** File paths,
  identifiers, and command names belong in
  inline backticks. Quoted prose belongs in
  block quotes. Code fences are for code.
- **Hashtag stacks.** Banned. The doc system has
  no use for them.
- **"Here are…" preambles.** *"Here are five
  ways to …"*, *"Below are the steps to…"*.
  Drop the preamble; lead with the list.

## Per-type allowances

A few patterns are slop *as default rhythm* and
fine *as one tool among many*. The skill applies
these per-type, not globally.

- **Reference pages** — bulleted lists with
  bolded label-and-colon openers are the canonical
  form for parameter tables. Allowed.
- **Background pages** — hedging language
  (*may, in some cases, often*) is allowed
  because the page legitimately discusses
  trade-offs. The seesaw construction
  (*"not just X, but Y"*) is still banned.
- **Tutorial pages** — a closing "next steps"
  block is fine; *"In conclusion,"* still is
  not.
- **How-to pages** — no allowances. How-tos are
  the strictest register: imperative, declarative,
  no decoration.

## False positives

Skip when the flagged word is one of these:

- A code identifier (`Navigate`, `Foster`,
  `Realm` — class or function names quoted
  verbatim).
- A direct quote from an external source.
- A glossary or reference entry defining the
  term itself ("**Tapestry** is a Java web
  framework…").

When in doubt, leave the prose and surface the
finding to the user as a question.
