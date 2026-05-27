---
diagnostics:
  - line: 3
    column: 1
    message: 'generated section directive has invalid "row-expr" expression: invalid cue expression: expected operand, found ''}'''
---
# Row-Expr With Invalid CUE

<?catalog
glob: "*.md"
row-expr: 'strings.Join([for x in}'
?>
<?/catalog?>
