---
settings:
  nested:
    - dash
    - asterisk
diagnostics:
  - line: 6
    column: 1
    message: "unordered list at depth 1 uses dash; expected asterisk"
---
# Bad nested with wrong inner marker

Outer uses dash (correct), inner should use asterisk but uses dash.

- Outer item
  - Inner item should be asterisk
  - Another inner item should be asterisk
- Another outer item
