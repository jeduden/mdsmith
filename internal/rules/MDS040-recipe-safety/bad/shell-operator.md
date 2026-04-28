---
settings:
  recipes:
    build:
      command: "make all && make install"
      params:
        required: []
diagnostics:
  - line: 1
    column: 1
    message: 'recipe "build": command contains shell operator "&&" — use a wrapper script'
---

# Shell Operator

A recipe whose command contains a shell operator.
