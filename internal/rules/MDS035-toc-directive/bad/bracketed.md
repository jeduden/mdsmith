---
diagnostics:
  - line: 3
    column: 1
    message: "unsupported TOC directive `[TOC]`; mdsmith has no heading TOC equivalent; use `<?catalog?>` for file indexes (MDS019)"
---
# Python-Markdown TOC directive

[TOC]

Everything below the directive renders fine,
but the directive itself appears as literal
text on CommonMark and goldmark renderers.
