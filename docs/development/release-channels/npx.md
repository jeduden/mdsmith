---
title: npx
summary: >-
  `npx @mdsmith/cli` runs mdsmith from the npm package
  with no global install, caching the platform binary
  after first use.
mechanism: toolchain
artifact: cli
command: npx @mdsmith/cli check .
audience: One-off checks without a global install
platforms: [node]
channelurl: https://www.npmjs.com/package/@mdsmith/cli
weight: 3
---
# npx

Release page: <https://www.npmjs.com/package/@mdsmith/cli>

`npx` runs mdsmith from the published npm package
without a global install. It downloads `@mdsmith/cli`
and the platform binary on first use, then caches
them. Later runs reuse the cache. This suits one-off
checks and CI steps.
