---
settings:
  style: dash
  nested: [dash, asterisk]
diagnostics:
  - line: 5
    column: 1
    message: "unordered list at depth 1 uses -; expected *"
---
# Title

- item one

  - nested one
  - nested two
