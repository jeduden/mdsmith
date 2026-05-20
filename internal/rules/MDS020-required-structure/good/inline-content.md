---
settings:
  inline-schema:
    closed: true
    sections:
      - heading: "Examples"
        content:
          - kind: code-block
            lang: yaml
          - kind: unlisted
      - heading: "Settings"
        content:
          - kind: table
            columns: [Setting, Default]
      - heading: "Steps"
        content:
          - kind: list
            ordered: true
            min-items: 2
            max-items: 5
      - heading: "Diagnosis"
        content:
          - kind: paragraph
---
# Runbook

## Examples

```yaml
foo: bar
```

Trailing notes are absorbed by the `unlisted` slot.

## Settings

| Setting | Default |
| ------- | ------- |
| timeout | 30s     |

## Steps

1. First step.
2. Second step.
3. Third step.

## Diagnosis

A paragraph names the diagnosis.
