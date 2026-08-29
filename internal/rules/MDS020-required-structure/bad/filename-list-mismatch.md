---
settings:
  schema: "../../internal/rules/MDS020-required-structure/bad/data/filename-list-tmpl.md"
diagnostics:
  - line: 1
    column: 1
    message: |-
      filename: got "filename-list-mismatch.md", expected filename matching one of globs [0-9]*_*.md, plan.md
---
# My Doc
