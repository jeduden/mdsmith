---
settings:
  inline-schema:
    frontmatter:
      nature: '"directive" | "generator" | "content" | "style" | "structure"'
diagnostics:
  - line: 1
    column: 1
    message: |-
      nature: got <missing>, expected one of: "directive", "generator", "content", "style", "structure"
---
# MDS999: example-rule

Body content lacking the nature key in front matter.
