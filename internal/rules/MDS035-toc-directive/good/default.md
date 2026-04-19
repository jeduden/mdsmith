---
settings:
diagnostics:
---
# Document with no TOC directives

This document has no renderer-specific TOC
markers, so MDS035 stays silent.

Normal prose is unaffected, and inline code
like `[TOC]` or `${toc}` is not flagged because
the tokens are inside code spans.

```text
[TOC]
[[_TOC_]]
[[toc]]
${toc}
```

Even the fenced block above is a code block,
not a paragraph, so nothing is reported.
