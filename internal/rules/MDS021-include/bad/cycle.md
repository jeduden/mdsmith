---
diagnostics:
  - line: 3
    column: 1
    message: "cyclic include: cycle.md -> data/cycle-b.md -> data/cycle-a.md -> data/cycle-b.md"
---
# Cycle Test

<?include
file: data/cycle-b.md
?>
old
<?/include?>
