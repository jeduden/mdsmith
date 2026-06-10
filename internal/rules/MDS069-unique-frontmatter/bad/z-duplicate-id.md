---
id: 7
settings:
  field: id
  include:
    - "**/*.md"
diagnostics:
  - line: 2
    column: 1
    message: 'front-matter "id": value 7 already used by ref/first.md'
---
# Duplicate id

This file repeats the id that ref/first.md already holds,
and it sorts later in path order, so it gets the
diagnostic.
