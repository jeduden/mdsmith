---
settings:
  recipes:
    unsafe:
      command: "../../scripts/run.sh {input}"
      params:
        required:
          - input
diagnostics:
  - line: 1
    column: 1
    message: 'recipe "unsafe": executable "../../scripts/run.sh" contains a .. path component'
---

# Path Traversal

A recipe whose executable token contains a .. path component.
