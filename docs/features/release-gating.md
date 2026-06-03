---
title: "Gate releases on doc status"
summary: >-
  `mdsmith list query 'status: "✅"' plan/` selects files by a CUE
  expression on front matter; `mdsmith metrics rank` ranks files by
  any shared metric — both ready to pipe into a release script.
icon: gauge
link: "/reference/cli/query/"
weight: 15
group: "Built for your pipeline"
---
# Gate releases on doc status

Front matter is data. mdsmith lets a release script query it.

`mdsmith list query 'status: "✅"' plan/` selects files by a
[CUE expression](../reference/cli/query.md) on front matter. It
prints the matching paths, so a script can block a release while
any plan is unfinished.

`mdsmith metrics rank --by token-estimate --top 10 docs/` ranks
files by any shared metric. Both commands emit plain lines, ready
to pipe into the rest of a pipeline.

See the [`list query`](../reference/cli/query.md) and
[`metrics`](../reference/cli/metrics.md) references for the full
expression grammar and metric list.
