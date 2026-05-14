---
diagnostics:
  - line: 3
    column: 1
    message: 'generated section directive has invalid "where" expression: invalid CUE expression: reference "nature" not found'
---
# Directive Documents

<?catalog
glob: "*.md"
where: 'nature == "directive"'
row: "- [{title}]({filename})"
?>
<?/catalog?>
