---
settings:
  recipes:
    render:
      command: "mmdc -i {input}"
      params:
        required:
          - input
        optional:
          - theme
diagnostics:
  - line: 1
    column: 1
    message: 'recipe "render": declared param "theme" is not referenced in command'
---

# Unused Parameter

A recipe that declares a param that is never referenced in the command.
