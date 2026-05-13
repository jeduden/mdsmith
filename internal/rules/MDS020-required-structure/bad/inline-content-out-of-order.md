---
settings:
  inline-schema:
    sections:
      - heading: "Examples"
        content:
          - kind: code-block
            lang: yaml
          - kind: table
            columns: [A, B]
diagnostics:
  - line: 5
    column: 1
    message: 'content "table" out of order: expected after "code-block lang=yaml"'
---
# Runbook

## Examples

| A | B |
|---|---|
| x | y |

```yaml
foo: bar
```
