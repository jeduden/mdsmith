---
settings:
  max-run: 2
  forbid-escaped-in-run: true
  forbid-adjacent-same-delim: true
diagnostics:
  - line: 3
    column: 1
    message: "emphasis run of 5 delimiters; max is 2"
  - line: 3
    column: 6
    message: "escaped delimiter inside emphasis run"
---
# Title

*****\*a* in the rant string.
