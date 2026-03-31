---
title: Rule Directory
summary: >-
  Complete list of all mdsmith rules with status and
  description, generated from rule READMEs.
---
# Rule Directory

All mdsmith rules. Each rule links to its full
README with parameters, examples, and diagnostics.

<?catalog
glob: "internal/rules/MDS*/README.md"
sort: id
header: |
  | Rule | Name | Status | Description |
  |------|------|--------|-------------|
row: "| [{id}]({filename}) | `{name}` | {status} | {description} |"
?>
<?/catalog?>
