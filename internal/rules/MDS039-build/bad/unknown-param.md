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
    message: 'build directive recipe "render": unknown parameter "bogus"'
---
# Unknown Param

<?build
recipe: render
source: diagram.svg
output: out.png
bogus: value
?>
![render output: out.png](out.png)
<?/build?>
