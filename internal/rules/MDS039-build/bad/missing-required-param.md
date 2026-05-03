---
settings:
  recipes:
    render:
      params:
        required:
          - source
diagnostics:
  - line: 3
    column: 1
    message: 'build directive recipe "render": missing required parameter "source"'
---
# Missing Required Param

<?build
recipe: render
output: out.png
?>
content
<?/build?>
