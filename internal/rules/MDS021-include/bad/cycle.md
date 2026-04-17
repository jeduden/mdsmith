---
diagnostics:
  - line: 3
    column: 1
    message: "data/cycle-b.md:3: data/cycle-a.md:3: cyclic include: cycle.md -> data/cycle-b.md -> data/cycle-a.md -> data/cycle-b.md"
---
# Cycle Test

<?include
file: data/cycle-b.md
?>
old
<?/include?>
