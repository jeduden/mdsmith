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

## Performance

MDS024 is **opt-in**. It is the single most expensive default
rule. The diagnostic needs exact sentence boundaries. The
per-sentence word count and the over-long sentence preview both
require them. mdsmith uses the trained Punkt sentence segmenter
(`github.com/neurosnap/sentences`). On prose-heavy input Punkt is
roughly 20% of mdsmith's wall time. The cost is the trained
model's regex execution. The hot function is
`english.MultiPunctWordAnnotation.tokenAnnotation`. It runs
`reAbbr` and the token-type matchers with backtracking. The pass
fires on every period-ending token.

The cheap upper-bound guard in [the rule source](./) skips Punkt
for paragraphs that provably cannot violate either limit. Short
paragraphs cost zero. The 20% is paid by paragraphs past the
guard. On prose-heavy input that is most of them.

No faster Go segmenter matches Punkt. See [plan 187][p187]
for the recorded negative. The harness at
[`sentence_equivalence_test.go`][harness] gates any future
candidate.

[p187]: ../../../plan/187_neutral-corpus-engine-lever.md
[harness]: ../../mdtext/sentence_equivalence_test.go

Enable when you want the diagnostic. Skip when you don't.

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
