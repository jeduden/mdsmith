---
id: MDS024
name: paragraph-structure
status: ready
description: Paragraphs must not exceed sentence and word limits.
category: prose
nature: content
maintainability: null
markdownlint: null
---
# MDS024: paragraph-structure

Paragraphs must not exceed sentence and word limits.

## Settings

| Setting                  | Type | Default | Description                                                                                                                |
|--------------------------|------|---------|----------------------------------------------------------------------------------------------------------------------------|
| `max-sentences`          | int  | 6       | Maximum sentences per paragraph                                                                                            |
| `max-words-per-sentence` | int  | 40      | Maximum words per sentence                                                                                                 |
| `placeholders`           | list | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Useful tokens: `var-token`, `heading-question`, `placeholder-section`.

Markdown tables and code blocks are skipped.

## How it works

MDS024 has two execution paths. A constant-time
**cheap-bounds guard** runs first. It proves a paragraph
cannot violate either limit. If the proof succeeds, the rule
returns. If not, the **exact path** runs the trained Punkt
sentence segmenter. The two paths share one invariant. The
guard skips Punkt only when the exact path would emit zero
diagnostics. The combined behavior is fast-or-exact, never
approximate.

### Exact diagnostics

When the rule fires, every number in the message is exact.

The sentence count comes from the trained Punkt segmenter
([`github.com/neurosnap/sentences`][punkt]). It classifies
every `.`/`!`/`?` against abbreviation, decimal, ellipsis,
and initial heuristics. It is not a regex-based count. The
per-sentence word count is the exact word count of the
Punkt-segmented sentence. It is not the paragraph total.
The over-long-sentence preview is a slice of the actual
Punkt-segmented sentence. It is not a guess.

A paragraph like "Dr. Smith met Mr. Jones at 3.14 p.m. on
Jan. 5." is one sentence, not seven. Naive splitters
disagree. Punkt is right. The rule reports what Punkt says.
So `paragraph has too many sentences (8 > 6)` means eight,
and `sentence too long (45 > 40 words): "..."` quotes the
real over-long sentence.

[punkt]: https://github.com/neurosnap/sentences

### Cheap-bounds guard

The guard runs one allocation-free pass over the paragraph.
It computes two bounds.

- ``sentenceUB = (count of `.`/`!`/`?`) + 1`` — an upper
  bound on Punkt's sentence count. Punkt only places
  boundaries at `.`/`!`/`?` and always yields at least one
  sentence. The bound is sound.
- `paragraphWords` — the exact whitespace-delimited word
  count for the whole paragraph. No single sentence has
  more words than the paragraph. The total is an upper
  bound on every sentence.

When both bounds are within the limits, Punkt cannot
fire. The rule returns early.
[`TestCheapBounds_GuardIsSound`][guard-test] pins this
invariant.

Short paragraphs, single-sentence paragraphs, and
lightly-punctuated paragraphs all clear the guard with
zero allocations. They contribute zero Punkt cost.

Placeholder masking runs before the guard. Configured
placeholder tokens (`{body}`, `{var-token}`, etc.)
collapse to neutral words. They never trip the cheap
path.

[guard-test]: ../paragraphstructure/rule_test.go

## Performance

MDS024 is **opt-in** by default. It is the most expensive
default rule when enabled. The exactness it guarantees
comes from real segmentation work. The cheap path avoids
that work. It cannot replace it.

Most paragraphs in prose-heavy input fail the cheap-bounds
guard. There Punkt is roughly 20% of mdsmith's wall time
([plan 187][p187] records the CPU profile). The hot frame
is `english.MultiPunctWordAnnotation.tokenAnnotation`. It
runs `reAbbr` and the token-type matchers with
backtracking on every period-ending token.
`regexp.(*Regexp).tryBacktrack` alone is 13% flat / 24%
cumulative of the segmenter's CPU.

No pure-Go Punkt-equivalent faster segmenter exists. The
negative is recorded in
[`sentence_equivalence_test.go`][harness]. The harness
gates any future candidate. A focused optimization
target — replacing `reAbbr` with a hand-rolled DFA scanner
— is tracked in [plan 191][p191].

The cheap-bounds guard bounds the cost. Only paragraphs
that *provably might* violate pay. Documentation,
code-heavy READMEs, and short-paragraph prose pay close
to zero. Long prose paragraphs that already trip the
limits pay full segmentation. That is what produces the
exact diagnostic the rule promises.

Enable when you want exact prose-structure diagnostics.
Skip when you don't.

[p187]: ../../../plan/187_neutral-corpus-engine-lever.md
[p191]: ../../../plan/191_punkt-reabbr-dfa.md
[harness]: ../../mdtext/sentence_equivalence_test.go

## Config

Enable with default thresholds:

```yaml
rules:
  paragraph-structure: true
```

Enable with custom thresholds:

```yaml
rules:
  paragraph-structure:
    max-sentences: 6
    max-words-per-sentence: 40
```

Explicitly disable (matches the default):

```yaml
rules:
  paragraph-structure: false
```

## Examples

### Good

<?include
file: good.md
wrap: markdown
?>

```markdown
# Well Structured Document

The sun rose over the hills. Birds began to sing.
A gentle breeze swept through the valley.
```

<?/include?>

### Bad

<?include
file: bad.md
wrap: markdown
?>

```markdown
# Overly Long Paragraph

Dogs bark. Cats meow. Birds sing. Fish swim. Frogs croak. Snakes hiss. Bees buzz. Ants march.
```

<?/include?>

## Diagnostics

| Condition          | Message                                    |
|--------------------|--------------------------------------------|
| too many sentences | `paragraph has too many sentences (8 > 6)` |
| sentence too long  | `sentence too long (45 > 40 words)`        |

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)

## Meta-Information

- **ID**: MDS024
- **Name**: `paragraph-structure`
- **Status**: ready
- **Default**: disabled (opt-in; see Performance above).
  When enabled: max-sentences: 6, max-words-per-sentence: 40
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: prose
