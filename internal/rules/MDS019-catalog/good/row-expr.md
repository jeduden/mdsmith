# Row-Expr Projection

<?catalog
glob: "row-expr-data/*.md"
sort: id
row-expr: |
  strings.Join(
    [for m in markdownlint {
      "\(m.id) \([if m.default {"✅"}, if !m.default {"⚪"}][0]) \(m.name)"
    }],
    ", "
  )
?>
MD013 ⚪ line-length
MD018 ✅ no-missing-space-atx, MD019 ✅ no-multiple-space-atx
<?/catalog?>
