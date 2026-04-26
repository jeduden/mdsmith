---
id: MDS028
name: token-budget
status: ready
description: File must not exceed a token budget.
---
# MDS028: token-budget

File must not exceed a token budget.

## Settings

| Setting           | Type   | Default       | Description                                                                          |
|-------------------|--------|---------------|--------------------------------------------------------------------------------------|
| `max`             | int    | 8000          | Default token budget when no per-glob budget matches                                 |
| `mode`            | string | `heuristic`   | Counting mode: `heuristic` or `tokenizer`                                            |
| `tokens-per-word` | number | 1.33          | Tokens per word multiplier used in `heuristic` mode                                  |
| `tokenizer`       | string | `builtin`     | Tokenizer family used in `tokenizer` mode                                            |
| `encoding`        | string | `cl100k_base` | Encoding profile for tokenizer mode: `cl100k_base`, `p50k_base`, `r50k_base`, `gpt2` |
| `budgets`         | list   | none          | Ordered per-glob budgets (`glob`, `max`); last matching entry wins                   |

## Config

Enable with defaults:

```yaml
rules:
  token-budget: true
```

Heuristic mode:

```yaml
rules:
  token-budget:
    mode: heuristic
    tokens-per-word: 1.33
    max: 2400
```

Tokenizer mode with per-glob budgets:

```yaml
rules:
  token-budget:
    mode: tokenizer
    tokenizer: builtin
    encoding: cl100k_base
    max: 3000
    budgets:
      - glob: "README.md"
        max: 4000
      - glob: "guides/*.md"
        max: 5000
```

Disable:

```yaml
rules:
  token-budget: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Token Budget

This file stays within budget.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Token Budget

one two three four five six
```

<?/include?>

## Meta-Information

- **ID**: MDS028
- **Name**: `token-budget`
- **Status**: ready
- **Default**: enabled, max: 8000, mode: heuristic, tokens-per-word: 1.33,
  tokenizer: builtin, encoding: cl100k_base
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta
