# Document Index

A `..` segment is allowed when the resolved pattern stays
inside the project root, mirroring how `<?include?>`
resolves its `file` parameter.

<?catalog
glob: "data/../data/*.md"
row: "[{filename}]({filename})"
?>
[data/alpha.md](data/alpha.md)
[data/beta.md](data/beta.md)
<?/catalog?>
