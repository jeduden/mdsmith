---
settings:
  path-patterns:
    - kind: plan
      pattern: "[0-9]*_*.md"
diagnostics:
  - line: 1
    column: 1
    message: |-
      path: got "path-pattern-mismatch.md", expected path matching glob [0-9]*_*.md
    related:
      - message: "kinds[plan] / path-pattern"
---
# Plan body whose path does not match the kind's path-pattern
