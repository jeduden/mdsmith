---
settings:
  inline-schema:
    sections:
      - heading: "Examples"
        closed: true
        content:
          - kind: code-block
diagnostics:
  - line: 9
    column: 1
    message: 'unexpected content "paragraph" inside ## Examples'
---
# Runbook

## Examples

```
x
```

Trailing paragraph triggers closed-scope unexpected diagnostic.
