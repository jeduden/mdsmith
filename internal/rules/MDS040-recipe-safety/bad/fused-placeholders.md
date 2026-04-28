---
settings:
  recipes:
    render:
      command: "tool {a}{b}"
      params:
        required:
          - a
          - b
diagnostics:
  - line: 1
    column: 1
    message: 'recipe "render": command contains fused placeholders "{a}{b}" — separate with a delimiter'
---

# Fused Placeholders

A recipe whose command contains two adjacent placeholders with no delimiter.
