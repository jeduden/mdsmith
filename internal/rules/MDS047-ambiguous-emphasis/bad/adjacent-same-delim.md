---
settings:
  max-run: 2
  forbid-escaped-in-run: true
  forbid-adjacent-same-delim: true
diagnostics:
  - line: 3
    column: 1
    message: "adjacent same-delimiter emphasis is ambiguous"
---
# Title

__a__b__ ambiguous between bold(a) literal-b and literal-a bold(b).
