---
settings:
  recipes:
    audit:
      command: "bash audit.sh"
diagnostics:
  - line: 1
    column: 1
    message: 'recipe "audit": command uses shell interpreter "bash" — use the direct binary'
---

# Shell Interpreter

A recipe that uses bash as the first token.
