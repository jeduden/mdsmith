---
settings:
  max: 50
  reflow: true
diagnostics:
  - line: 3
    column: 51
    message: "line too long (136 > 50)"
  - line: 5
    column: 51
    message: "line too long (179 > 50)"
  - line: 7
    column: 51
    message: "line too long (105 > 50)"
---
# Block starts

Diagnostics stopped being tied to the login prompt in [Issue #48](https://example.com/repo/issues/48), and the flags were removed later.

When writing Markdown, a line that starts with # and a space opens a heading, one that starts with > opens a block quote, and a line that begins with 1. and a space starts a list.

The install steps in this guide are numbered from 1. Step 2 builds the binary, and step 3 runs the tests.
