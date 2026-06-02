---
settings:
  schema: "../../internal/rules/MDS020-required-structure/bad/data/filename-tmpl.md"
diagnostics:
  - line: 1
    column: 1
    message: |-
      filename: got "filename-mismatch.md", expected filename matching glob [0-9]*_*.md
---
# My Doc
