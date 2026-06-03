---
title: Obsidian
summary: >-
  A `mdsmith-obsidian-<version>.zip` of the WebAssembly
  engine plus the plugin's five load files, attached to
  each GitHub release and installed by unzipping into a
  vault's `.obsidian/plugins/mdsmith/` folder.
mechanism: push
artifact: obsidian-plugin
command: unzip mdsmith-obsidian-<version>.zip -d <vault>/.obsidian/plugins/mdsmith/
audience: Obsidian vaults on desktop and mobile
platforms: [editor]
registry: github.com/jeduden/mdsmith/releases
credential: GITHUB_TOKEN + OIDC
job: obsidian
channelurl: https://github.com/jeduden/mdsmith/releases
weight: 14
---
# Obsidian

Release page: <https://github.com/jeduden/mdsmith/releases>

The Obsidian channel ships the plugin as a single
`mdsmith-obsidian-<version>.zip` attached to each GitHub
release. The zip holds the five files Obsidian loads, flat:
`main.js`, `manifest.json`, `styles.css`, `mdsmith.wasm`,
and `wasm_exec.js`. The engine is compiled to WebAssembly.
So one artifact runs on every platform Obsidian supports.
There is no per-platform build matrix and no native binary.

The plugin is **not** in the Obsidian community catalog
(plan 217 Non-Goals). GitHub Releases is its only channel,
so a user installs it by hand:

1. Download `mdsmith-obsidian-<version>.zip` from the
   release page.
2. Create `<vault>/.obsidian/plugins/mdsmith/`.
3. Unzip the five files into that folder.
4. In Obsidian, open **Settings → Community plugins** and
   enable **mdsmith**.

The user-facing
[mdsmith for Obsidian](../../guides/editors/obsidian.md)
guide covers the same steps plus the settings and
troubleshooting.

The `obsidian` job in `release.yml` builds the artifact. It
does not depend on the `build` matrix — there is one WASM
artifact, not a per-platform binary. The job stamps the
release version into the tracked manifests with
[`stamp`](../release-tooling.md) (the plugin's
`manifest.json` and `package.json` are in the tracked set),
compiles the engine with `cmd/mdsmith-wasm/build.sh`, builds
the plugin with `bun run build.ts --production`, and packs
`dist/` with [`package-obsidian`](../release-tooling.md).
That command reads the version from the stamped
`manifest.json` and writes `mdsmith-obsidian-<version>.zip`
through Go's `archive/zip`, so the channel needs no `zip`
binary. The zip stays inside the
[WASM size budget](../../background/concepts/engine-api.md).

The `release` job attaches that artifact to the draft. The
zip name matches the `mdsmith-*` glob the release job
already uses, so `checksums.txt`, the SLSA build-provenance
attestation, and the cosign signature cover it the same as
the raw binaries.

Auth: none of its own. The zip rides the `release` job's
`GITHUB_TOKEN` upload and OIDC signing — the same path the
other release artifacts take. There is no publisher token to
rotate and no `release` environment gate, because the only
channel is the GitHub release itself.
