---
diagnostics:
  - line: 3
    column: 1
    message: generated section is out of date
---
# Heading Offset Example

<?include
file: data/offset-src.md
heading-offset: "1"
?>
Overview of the included content goes here first.

# Primary

Body for the primary section with a few words.

## Secondary

Body for the secondary section with a few words.
<?/include?>
