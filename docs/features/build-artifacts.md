---
title: "Build artifacts in sync"
summary: >-
  The `<?build?>` directive declares an artifact and a recipe.
  `mdsmith fix` keeps the section body in sync with the recipe
  output; `MDS040` shell-safety-checks the recipe without running it.
icon: blocks
link: "/guides/directives/build/"
rules: ["MDS039", "MDS040"]
weight: 14
group: "Markdown as a single source of truth"
---
# Build artifacts in sync

A README often quotes a generated file: a help dump, a config
sample, a version table. That copy goes stale the moment the
source changes.

The `<?build?>` directive declares an artifact and a recipe from
`build.recipes`. On `mdsmith fix`, the section body is rendered
from the recipe's `body-template`, so the doc and the artifact
can never drift. `MDS039` validates the directive parameters.

Recipes are inspected, not trusted. `MDS040` statically checks
every recipe command for shell-safety at lint time and never
executes a binary itself.

## Incremental rebuilds

The build pass is incremental. `mdsmith fix` hashes each target's
recipe spec, input contents, and output set into one ActionID and
rebuilds only the targets whose ActionID changed or whose outputs
are missing or hand-edited. A fresh target prints `SKIP`; freshness
is tracked in `.mdsmith/build-cache.json`. `--build-check-stale`
turns that into a CI gate, exiting non-zero when any artifact is out
of date without running a recipe.

The build cache and working directories are machine-local. Add them
to `.gitignore`. Never ignore the whole `.mdsmith/` folder: its
`kinds/`, `schemas/`, and `conventions/` subfolders are checked-in
config.

```text
.mdsmith/build-cache.json
.mdsmith/build-logs/
.mdsmith/build-staging/
```

See the [build directive guide](../guides/directives/build.md)
for recipe declaration and the body-template syntax.
