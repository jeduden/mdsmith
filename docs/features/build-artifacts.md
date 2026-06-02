---
title: "Build artifacts in sync"
summary: >-
  The `<?build?>` directive declares an artifact and a recipe.
  `mdsmith fix` keeps the section body in sync with the recipe
  output; `MDS040` shell-safety-checks the recipe without running it.
icon: blocks
link: "/guides/directives/build/"
rules: ["MDS039", "MDS040"]
weight: 13
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

See the [build directive guide](../guides/directives/build.md)
for recipe declaration and the body-template syntax.
