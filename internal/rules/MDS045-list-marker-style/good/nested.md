---
settings:
  nested:
    - dash
    - asterisk
---
# Good nested with rotation

Outer lists use dash, inner use asterisk.

- Outer item
  * Inner item
  * Another inner item
- Another outer item
  * More inner
    - Depth 2 cycles back to dash
      * Depth 3 cycles to asterisk
