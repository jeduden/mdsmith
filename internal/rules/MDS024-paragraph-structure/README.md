---
id: MDS024
name: paragraph-structure
status: ready
description: Paragraphs must not exceed sentence and word limits.
category: prose
nature: content
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
---
# MDS024: paragraph-structure

Paragraphs must not exceed sentence and word limits.

## Settings

| Setting                  | Type | Default | Description                                                                                                                |
| ------------------------ | ---- | ------- | -------------------------------------------------------------------------------------------------------------------------- |
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

MDS024 is **opt-in** by default. Short and
lightly-punctuated paragraphs clear the cheap-bounds
guard at zero allocations. Long paragraphs that the
guard cannot rule out pay full Punkt segmentation.

The segmenter is a fork of upstream Punkt vendored
under [`internal/punkt/`](../../punkt/). The fork is
byte-identical to upstream over the equivalence
corpus, but allocation-clean per call. A warm Check
call against an abbr-heavy paragraph runs at ≤ 10
allocs/op — pinned by `BenchmarkRule_MDS024` and
matching the per-rule budget in
[CLAUDE.md](../../../CLAUDE.md). The `-tags
mdtext_punkt_upstream` build keeps the original
pipeline around for A/B comparison.

Plan
[193](../../../plan/193_mds024-allocation-budget.md)
records the rework and the measured before/after
numbers.

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
file: good/english.md
wrap: markdown
?>

```markdown
# Well Structured Document

The sun rose over the hills. Birds began to sing.
A gentle breeze swept through the valley.
```

<?/include?>

<?include
file: good/chinese.md
wrap: markdown
?>

```markdown
# Well Structured Chinese Document

太阳升起。鸟儿开始歌唱。一阵微风吹过山谷。
```

<?/include?>

<?include
file: good/japanese.md
wrap: markdown
?>

```markdown
# Well Structured Japanese Document

太陽が昇る。鳥が歌い始める。風が谷を吹き抜ける。
```

<?/include?>

### Bad

<?include
file: bad/english.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Overly Long Paragraph

Dogs bark. Cats meow. Birds sing. Fish swim. Frogs croak. Snakes hiss. Bees buzz. Ants march.
```

<?/include?>

<?include
file: bad/chinese.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Overly Long Chinese Paragraph

今天天气很好。鸟儿在歌唱。微风轻拂。阳光明媚。空气清新。花朵盛开。山谷宁静。
```

<?/include?>

The segmenter treats the full-width Chinese / Japanese period
`。` as a sentence boundary the same way it does ASCII `.`, so
the rule fires on CJK paragraphs that end every sentence with
`。`. Mixed CJK / ASCII paragraphs work too.

Full-width `！` and `？` are word boundaries in the trained
English pipeline (the vendored Punkt fork inherits that), but
they do not flag a sentence break on their own — match
upstream behaviour. Author CJK paragraphs with `。` between
sentences for the rule to count them correctly.

## Diagnostics

| Condition          | Message                                    |
| ------------------ | ------------------------------------------ |
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
