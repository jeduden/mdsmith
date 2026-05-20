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
sentence segmenter. The guard skips the segmenter only when
the exact path would emit zero diagnostics. The combined
behavior is fast-or-exact, never approximate.

### Exact diagnostics

When the rule fires, every number in the message is exact.

The sentence count comes from the trained Punkt segmenter
([`github.com/neurosnap/sentences`][punkt]). It classifies
every `.`/`!`/`?` against abbreviation, decimal, ellipsis,
and initial heuristics — not a regex-based count. The
per-sentence word count is the exact word count of the
Punkt-segmented sentence (not the paragraph total). The
over-long-sentence preview is a slice of the actual
Punkt-segmented sentence (not a guess).

A paragraph like "Dr. Smith met Mr. Jones at 3.14 p.m. on
Jan. 5." is one sentence, not seven. Naive splitters
disagree. Punkt is right. So `paragraph has too many
sentences (8 > 6)` means eight, and `sentence too long
(45 > 40 words): "..."` quotes the real over-long sentence.

[punkt]: https://github.com/neurosnap/sentences

### Cheap-bounds guard

The guard runs one allocation-free pass over the paragraph
and computes two bounds.

- ``sentenceUB = (count of `.`/`!`/`?`) + 1`` — an upper
  bound on Punkt's sentence count. Punkt only places
  boundaries at `.`/`!`/`?` and always yields at least one
  sentence.
- `paragraphWords` — the exact whitespace-delimited word
  count for the whole paragraph. No single sentence has
  more words than the paragraph.

When both bounds are within the limits, the segmenter
cannot fire, so the rule returns early. Short paragraphs
and lightly-punctuated paragraphs clear the guard at zero
allocations.

Placeholder masking runs before the guard. Configured
placeholder tokens (`{body}`, `{var-token}`, etc.) collapse
to neutral words and never trip the cheap path.

## Performance

MDS024 is **opt-in** by default — and is the most expensive
rule mdsmith ships once you enable it. Most short and
lightly-punctuated paragraphs clear the cheap-bounds guard
at zero allocations and contribute zero segmenter cost. Long
prose paragraphs that the guard cannot rule out pay full
Punkt segmentation, which is the price of an exact diagnostic.

Enable when you want exact prose-structure diagnostics.
Skip when you don't.

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
