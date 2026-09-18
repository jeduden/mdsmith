---
settings:
  path-patterns:
    - kind: apm-skill
      pattern: "skills/\\#(fmvar(name))/SKILL.md"
diagnostics:
  - line: 1
    column: 1
    message: |-
      path: got "path-pattern-fmvar-missing.md", expected path matching glob skills/\#(fmvar(name))/SKILL.md
        (`fmvar(name)`: frontmatter value missing)
    related:
      - message: "kinds[apm-skill] / path-pattern"
---
# Document whose path-pattern interpolates an absent front-matter field
