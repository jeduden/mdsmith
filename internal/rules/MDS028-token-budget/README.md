---
id: MDS028
name: token-budget
description: File must not exceed a token budget.
---
# MDS028: token-budget

File must not exceed a token budget.

- **ID**: MDS028
- **Name**: `token-budget`
- **Default**: enabled, max: 8000, mode: heuristic, ratio: 0.75,
  tokenizer: builtin, encoding: cl100k_base
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting   | Type   | Default     | Description                                                                |
|-----------|--------|-------------|----------------------------------------------------------------------------|
| `max`       | int    | 8000        | Default token budget when no per-glob budget matches                       |
| `mode`      | string | `heuristic`   | Counting mode: `heuristic` or `tokenizer`                                      |
| `ratio`     | number | 0.75        | Tokens per word multiplier used in `heuristic` mode                          |
| `tokenizer` | string | `builtin`     | Tokenizer family used in `tokenizer` mode                                    |
| `encoding`  | string | `cl100k_base` | Encoding profile for tokenizer mode: `cl100k_base`, `p50k_base`, `r50k_base`, `gpt2` |
| `budgets`   | list   | none        | Ordered per-glob budgets (`glob`, `max`); last matching entry wins             |

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
    ratio: 0.75
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

```markdown
# Short Doc

This file is well under the configured token budget.
```

### Bad

```text
file.md:1:1 MDS028 token budget exceeded (4150 > 4000, mode=tokenizer:builtin/cl100k_base)
```
