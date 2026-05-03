---
settings:
  recipes:
    render:
      body-template: "![{alt}]({output})"
      params:
        required:
          - source
---
# Any Extension

<?build
recipe: render
source: demo.tape
output: demo.mp4
?>
![render output: demo.mp4](demo.mp4)
<?/build?>
