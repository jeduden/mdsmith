---
settings:
  recipes:
    render:
      body-template: "![{alt}]({output})"
      params:
        required:
          - source
diagnostics:
  - line: 3
    column: 1
    message: "generated section is out of date"
---
# Stale Body

<?build
recipe: render
source: diagram.svg
output: docs/diagram.png
?>
outdated content here
<?/build?>
