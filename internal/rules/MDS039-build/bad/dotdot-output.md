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
    message: 'build directive "output" contains ".." path component: "../out/file.png"'
---
# Dotdot Output

<?build
recipe: render
source: diagram.svg
output: ../out/file.png
?>
content
<?/build?>
