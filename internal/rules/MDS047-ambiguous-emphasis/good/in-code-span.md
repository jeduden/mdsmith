---
settings:
  max-run: 2
  forbid-escaped-in-run: true
  forbid-adjacent-same-delim: true
---
# In code span

The string `*****\*a*` inside an inline code span must not flag, and
neither must `__a__b__` nor `***Peter* Piper**` when wrapped this way.
