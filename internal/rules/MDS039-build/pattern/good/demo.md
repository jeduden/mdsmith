---
settings:
  recipes:
    vhs:
      body-template: "![demo]({output})"
      params:
        required:
          - source
---
# Demo

<?build
recipe: vhs
source: demo.tape
output: demo.gif
?>
![demo](demo.gif)
<?/build?>
