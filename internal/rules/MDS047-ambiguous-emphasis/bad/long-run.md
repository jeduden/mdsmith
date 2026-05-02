---
settings:
  max-run: 2
  forbid-escaped-in-run: true
  forbid-adjacent-same-delim: true
diagnostics:
  - line: 3
    column: 1
    message: "emphasis run of 3 delimiters; max is 2"
---
# Title

***bold-italic*** at the start of a paragraph.
