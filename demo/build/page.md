# Build directive demo

The `copy` recipe copies `source.txt` to `artifact.txt` when you run
`mdsmith fix`.

<?build
recipe: copy
inputs:
  - source.txt
outputs:
  - artifact.txt
?>
[artifact.txt](artifact.txt)
<?/build?>
