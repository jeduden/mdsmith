---
settings:
  path-patterns:
    - kind: plan
      pattern: "[0-9]*_*.md"
diagnostics:
  - line: 1
    column: 1
    message: |-
      filename: got "path-pattern-mismatch.md", expected
        glob [0-9]*_*.md
      schema: kinds[plan] / path-pattern
---
# Plan body whose path does not match the kind's path-pattern
