---
settings:
  nested:
    - dash
    - asterisk
---
# Bad nested with wrong inner marker

Outer uses dash (correct), inner should use asterisk but uses dash.

- Outer item
  * Inner item should be asterisk
  * Another inner item should be asterisk
- Another outer item
