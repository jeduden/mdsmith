---
settings:
  recipes:
    mermaid:
      command: "mmdc -i {input} -o {output}"
      params:
        required:
          - input
        optional:
          - output
---
# Safe Recipe

A recipe whose command uses a direct binary with no shell operators,
no fused placeholders, and no path traversal.
