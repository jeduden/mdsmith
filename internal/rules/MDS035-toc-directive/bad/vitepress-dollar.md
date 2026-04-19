---
diagnostics:
  - line: 3
    column: 1
    message: "unsupported TOC directive `${toc}`; mdsmith has no heading TOC equivalent; use `<?catalog?>` for file indexes (MDS019)"
---
# VitePress dollar-brace TOC directive

${toc}

Some VitePress configurations expand this
token. CommonMark and goldmark render it as
literal text.
