---
settings:
  recipes:
    render:
      command: "mmdc -i {input} -o {outputs}"
      params:
        required:
          - input
          - outputs
diagnostics:
  - line: 1
    column: 1
    message: 'recipe "render": reserved parameter name "outputs" must not be declared in params'
---

# Reserved Parameter Name

A recipe must not declare `outputs` as a named param; it is the
collective placeholder expanded from the directive's `outputs:` list.
