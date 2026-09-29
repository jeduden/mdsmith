---
settings:
  scope: section
  max: 3
  min-length: 4
diagnostics:
  - line: 1
    column: 1
    message: '"process" repeated 8 time(s) in section (max 3)'
  - line: 5
    column: 1
    message: '"process" repeated 4 time(s) in section (max 3)'
  - line: 9
    column: 1
    message: '"process" repeated 4 time(s) in section (max 3)'
---
# A

process process process process.

## B

process process process process.

# C

process process process process.
