---
id: MDS026
name: paragraph-conciseness
description: Paragraph verbosity score must not exceed a threshold.
---
# MDS026: paragraph-conciseness

Paragraph verbosity score must not exceed a threshold.

- **ID**: MDS026
- **Name**: `paragraph-conciseness`
- **Default**: enabled, max-verbosity: 30.0,
  min-words: 20, min-content-ratio: 0.45
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting           | Type  | Default       | Description                                         |
|-------------------|-------|---------------|-----------------------------------------------------|
| `max-verbosity`     | float | 30.0          | Maximum allowed verbosity score on a 0 to 100 scale |
| `min-words`         | int   | 20            | Minimum word count to check a paragraph             |
| `min-content-ratio` | float | 0.45          | Minimum content-word ratio before penalty           |
| `filler-words`      | list  | built-in list | Single-word filler terms to penalize                |
| `hedge-phrases`     | list  | built-in list | Hedge terms and phrases to penalize                 |
| `verbose-phrases`   | list  | built-in list | Verbose multi-word phrases to penalize              |

The rule computes a paragraph verbosity score from:

1. Filler-word density
2. Hedge-phrase density
3. Verbose-phrase density
4. Content-to-token ratio (lexical density)

Diagnostics include verbosity, conciseness (`100 - verbosity`), and example
phrases to trim.

## Config

```yaml
rules:
  paragraph-conciseness: true
```

Custom thresholds and heuristic lists:

```yaml
rules:
  paragraph-conciseness:
    max-verbosity: 30.0
    min-words: 24
    min-content-ratio: 0.5
    filler-words:
      - basically
      - really
    hedge-phrases:
      - in most cases
      - to some extent
    verbose-phrases:
      - in order to
      - it is important to note that
```

Disable:

```yaml
rules:
  paragraph-conciseness: false
```

## Examples

### Good

```markdown
The build command validates markdown files, reports exact line numbers,
and links each diagnostic to a rule so contributors can fix issues quickly.
```

### Bad

```markdown
In order to make sure that everyone is on the same page, it is important
to note that this is basically a very simple update that we might adjust
later in most cases.
```
