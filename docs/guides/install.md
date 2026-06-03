---
title: Installation
weight: 10
summary: >-
  Every channel that ships the mdsmith binary, the VS
  Code extension, or the Claude Code plugin — npm,
  PyPI, Homebrew, asdf, mise, a Flatpak bundle, the
  GitHub release, the Visual Studio Marketplace plus
  Open VSX, and the in-repository Claude Code
  marketplace — and which channel to pick for which
  workflow.
---
# Installation

## Quick start (CLI only)

For a minimal setup, install the binary and run it. No
config file, no kinds or schemas, no LSP, no editor
plugin required:

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
mdsmith fix README.md
mdsmith check .
```

That works on a single-file README repo as well as on a
600-file docs monorepo. `.mdsmith.yml` is optional — the
built-in defaults apply when no config is present. The
rest of this guide covers the other channels, the
editor extensions, and the supply-chain verification
steps for regulated deployments.

## Channels

Each `vX.Y.Z` git tag ships the same Go binary through
several channels. `mdsmith version` reports the same
value on every channel because the version is stamped
into the binary at build time. Pick one path:

<?catalog
glob:
  - "../development/release-channels/*.md"
  - "!../development/release-channels/proto.md"
where: 'artifact: "cli"'
sort: numeric:weight
header: |
  | Channel | Command | Best for |
  | ------- | ------- | -------- |
row: "| {title} | `{command}` | {audience} |"
?>
| Channel         | Command                                                                                    | Best for                                          |
| --------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------- |
| Go              | `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`                                 | Go developers with a working Go toolchain         |
| npm             | `npm install -g @mdsmith/cli`                                                              | Node / TypeScript repos and npm-friendly CI       |
| npx             | `npx @mdsmith/cli check .`                                                                 | One-off checks without a global install           |
| PyPI            | `pip install mdsmith`                                                                      | Python projects and Python-only CI images         |
| uvx             | `uvx mdsmith check .`                                                                      | Ephemeral runs via uv                             |
| pipx            | `pipx install mdsmith`                                                                     | Isolated CLI install on Python hosts              |
| Homebrew        | `brew install jeduden/mdsmith/mdsmith`                                                     | macOS and Linux via Homebrew                      |
| mise            | `mise use -g ubi:jeduden/mdsmith@latest`                                                   | Repos using mise; works today via GitHub releases |
| asdf            | `asdf plugin add mdsmith https://github.com/jeduden/asdf-mdsmith.git`                      | Repos standardized on asdf                        |
| GitHub Releases | `curl -LO https://github.com/jeduden/mdsmith/releases/latest/download/mdsmith-<os>-<arch>` | Air-gapped hosts and direct binary control        |
| Flatpak         | `flatpak install ./mdsmith-x86_64.flatpak`                                                 | Sandboxed Linux x86_64 desktops via Flatpak       |
<?/catalog?>

A bare `mise use mdsmith@latest` needs a registry entry —
"bare" meaning no backend prefix and no prior
`mise plugins install`. The short `asdf plugin add mdsmith`
needs one too. Both are tracked in
[plan/145](../../plan/145_asdf-mise-registry-submissions.md).
Everything else works today: the explicit-URL asdf install,
Homebrew, and every backend-prefixed mise form below.

The binary ships for linux x86_64, linux aarch64, macOS
x86_64, macOS arm64, and Windows amd64. Other targets
require a Go toolchain.

## npm

```bash
npm install -g @mdsmith/cli
mdsmith version
```

The npm root is published as `@mdsmith/cli` (the
unscoped `mdsmith` name on npm is owned by another
project; we use the `@mdsmith` scope we own
instead). The installed binary is still called
`mdsmith` because the package's `bin` field maps
the command to a small Node.js shim.

The shim declares `optionalDependencies` for one
platform sub-package per supported host (the canonical
list lives in [release-channels/npm.md][npm-channel]);
npm installs only the one that matches
`process.platform` and `process.arch`.

[npm-channel]: ../development/release-channels/npm.md
There is no `postinstall` hook, so `npm install`
works in offline / air-gapped CI and on hosts that
ban network calls during install.

`npx @mdsmith/cli` and `pnpm dlx @mdsmith/cli` work
the same way without a permanent install.

## PyPI (pip / uvx / pipx)

```bash
pip install mdsmith
mdsmith version
```

```bash
uvx mdsmith check .
```

The PyPI release ships one platform-tagged wheel per
supported host. Each wheel bundles the prebuilt
binary under `mdsmith/_bin/` and exposes an `mdsmith`
console script that runs the binary in place: `os.execv`
on POSIX (so signals and exit codes pass through
unchanged) and `subprocess.run` on Windows, which has
no `execv` semantics. `pip`, `uv pip`, `pipx`, `uvx`,
and `python -m mdsmith` all work.

## asdf

The [`jeduden/asdf-mdsmith`](https://github.com/jeduden/asdf-mdsmith)
plugin installs the prebuilt binary for your platform and verifies it
against the release `checksums.txt`:

```bash
asdf plugin add mdsmith https://github.com/jeduden/asdf-mdsmith.git
asdf install mdsmith latest
asdf set mdsmith latest
mdsmith version
```

Once the plugin is also listed in
[`asdf-vm/asdf-plugins`](https://github.com/asdf-vm/asdf-plugins),
the explicit URL becomes optional:
`asdf plugin add mdsmith` resolves on its own.

## Homebrew

```bash
brew install jeduden/mdsmith/mdsmith
mdsmith version
```

This taps
[`jeduden/homebrew-mdsmith`](https://github.com/jeduden/homebrew-mdsmith)
and installs the prebuilt binary for macOS or Linux, on Intel or
arm64, verified against the release `checksums.txt`. Upgrade with
`brew upgrade mdsmith`. To tap once and then install by short name:

```bash
brew tap jeduden/mdsmith
brew install mdsmith
```

## mise

```bash
mise use -g github:jeduden/mdsmith
mdsmith version
```

mise installs mdsmith through any of its tool backends —
all of these resolve today, none need a registry submission.
The `github` backend above reads our release assets directly
and auto-selects the build for your OS and architecture; pin
a version with `github:jeduden/mdsmith@0.13.2`.

To reuse the same plugin the asdf install uses:

```bash
mise plugins install mdsmith https://github.com/jeduden/asdf-mdsmith.git
mise use -g mdsmith@latest
```

To build from source with the Go backend (needs a Go
toolchain):

```bash
mise use -g go:github.com/jeduden/mdsmith/cmd/mdsmith
```

The `ubi` backend still works too, though mise has deprecated
it for new registry entries:

```bash
mise use -g ubi:jeduden/mdsmith@latest
```

The one gap is a bare `mise use mdsmith@latest`: no backend
prefix, and no prior `mise plugins install` like the asdf
step above. That form needs an entry in mise's curated
registry ([`jdx/mise`](https://github.com/jdx/mise)
`registry.toml`), tracked in
[plan/145](../../plan/145_asdf-mise-registry-submissions.md).
Until it merges, use a backend-prefixed form, or install the
plugin first as shown above.

## Flatpak

mdsmith ships an **x86_64-only** Flatpak bundle as a release
asset. Download `mdsmith-x86_64.flatpak` from the
[release page](https://github.com/jeduden/mdsmith/releases),
then install and run it:

```bash
flatpak install ./mdsmith-x86_64.flatpak
flatpak run io.github.jeduden.mdsmith check .
```

The first install also pulls the `org.freedesktop.Platform`
24.08 runtime from Flathub, if your host lacks it. Flatpak runs
the linter in a sandbox. The bundle grants `--filesystem=host`,
so mdsmith can read a repository anywhere on disk.

Invoke the linter through `flatpak run
io.github.jeduden.mdsmith`. Or add the Flatpak exports directory
to your `PATH` for a bare `mdsmith` command. aarch64 Linux hosts
use npm, PyPI, or the GitHub release binary instead.

## GitHub release (direct download)

The [release page](https://github.com/jeduden/mdsmith/releases)
attaches one binary per platform and a
`checksums.txt`. Download, verify the SHA-256, and
move the binary onto `$PATH`:

```bash
base="https://github.com/jeduden/mdsmith/releases/latest/download"
curl -L -o mdsmith-linux-amd64 "$base/mdsmith-linux-amd64"
curl -L -o checksums.txt       "$base/checksums.txt"
sha256sum -c <(grep mdsmith-linux-amd64 checksums.txt)
install -m 0755 mdsmith-linux-amd64 /usr/local/bin/mdsmith
```

Keep the binary saved under its release-asset name
(`mdsmith-linux-amd64`) until verification is done —
both `sha256sum -c` and `gh attestation verify` below
match local files against that exact name. `install`
copies the file rather than moving it, so the original
remains for the verification steps.

For supply-chain-sensitive deployments, the release
also ships a SLSA build provenance attestation per
binary and a Sigstore signature on `checksums.txt`.
Verify the provenance with `gh`:

```bash
gh attestation verify mdsmith-linux-amd64 \
  -R jeduden/mdsmith
```

Verify the checksums-file signature with `cosign`
(requires cosign v3.0.0 or newer — earlier versions
do not accept `verify-blob --bundle`; check yours
with `cosign version`):

```bash
curl -L -o checksums.txt.bundle "$base/checksums.txt.bundle"
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp \
    "^https://github.com/jeduden/mdsmith/.github/workflows/release.yml@" \
  --certificate-oidc-issuer \
    https://token.actions.githubusercontent.com \
  checksums.txt
```

Both verifications resolve through the workflow's
GitHub OIDC identity, so a forged binary or rewritten
checksums file fails verification unless the attacker
also controls `release.yml` on `jeduden/mdsmith`.

### CycloneDX SBOM

Every release also publishes a CycloneDX SBOM of the
Go module the binaries were built from. The file is
named `mdsmith-sbom.cdx.json`. The SHA-256 hash is
in `checksums.txt` (it matches the `mdsmith-*` glob),
so the same cosign signature transitively covers it.

```bash
curl -L -o mdsmith-sbom.cdx.json "$base/mdsmith-sbom.cdx.json"
sha256sum -c <(grep mdsmith-sbom.cdx.json checksums.txt)
```

Feed the SBOM into your dependency-scanner of choice
(`grype`, `osv-scanner`, your SCA platform) for a
component-and-license inventory of the binary.

This path is also the documented fallback if any of
the package channels above is unavailable on a given
day.

## VS Code extension

The extension talks to the Go binary over LSP. Install
the binary by one of the channels above, then add the
extension:

- **Visual Studio Marketplace** (stock VS Code, GitHub
  Codespaces, GitHub.dev): search for `jeduden.mdsmith`
  in the Extensions view, or run
  `code --install-extension jeduden.mdsmith`.
- **Open VSX** (VSCodium, Cursor, Theia, Gitpod):
  install from the marketplace UI, or run
  `codium --install-extension jeduden.mdsmith`.
- **GitHub release**: download the `.vsix` from the
  release page and run
  `code --install-extension mdsmith-X.Y.Z.vsix`.

The Marketplace, Open VSX, and GitHub-release `.vsix`
have identical SHA-256 sums; they're the same
artifact uploaded to three places.

See [VS Code Integration](editors/vscode.md) for the
configuration surface (`mdsmith.path`, `mdsmith.config`,
`mdsmith.run`, `mdsmith.previewFix`, and
`mdsmith.trace.server`) and fix-on-save via
`editor.codeActionsOnSave`.

## Claude Code plugins

The Claude Code plugin is an optional editor surface.
mdsmith itself never calls an LLM or any external
service at runtime; the plugin only spawns `mdsmith
lsp` as a local subprocess. See the
[telemetry policy](../reference/telemetry.md) for the
full statement.

The mdsmith marketplace ships six plugins.
Register once, then install whichever you need:

```text
/plugin marketplace add jeduden/mdsmith
```

### mdsmith-lsp

Inline diagnostics, quick-fixes, and navigation
(definition, references, symbol search,
call-hierarchy) via `mdsmith lsp`. Required by
most users.

```text
/plugin install mdsmith-lsp@mdsmith
/reload-plugins
```

The plugin runs `npx -y -p @mdsmith/cli mdsmith
lsp`. `npx` is bundled with npm, which standard
Node.js installers ship and Claude Code already
requires. First launch downloads `@mdsmith/cli`
and the platform binary from npm; later launches
reuse the cache. No global `mdsmith` install
needed. To pin a version, edit the manifest
`args` to `@mdsmith/cli@<ver>`.

If the `/plugin` Errors tab shows `Executable
not found in $PATH`, `npx` is missing. Install
Node 18 or later (20 LTS recommended) and run
`/reload-plugins`.

See the
[Claude Code editor README](../../editors/claude-code/README.md)
for troubleshooting steps.

### mdsmith-skills

Three slash-command skills:
`/mdsmith-fix [path]`, `/mdsmith-kinds [...]`,
and `/mdsmith-check [path]`. Useful without the
LSP plugin, or to run a targeted fix from the
command palette.

```text
/plugin install mdsmith-skills@mdsmith
/reload-plugins
```

Requires `mdsmith` on the `$PATH` Claude Code
sees (or the mdsmith source tree under
`./cmd/mdsmith`).

### mdsmith-reviewer

A `markdown-reviewer` subagent that reviews
Markdown PRs and drafts for structural drift.
Loads rule-backed patterns from `mdsmith help
patterns` at review time and checks three
config-level patterns from a sibling
`patterns.md`. Proposes config or directive
snippets; no auto-fix.

```text
/plugin install mdsmith-reviewer@mdsmith
/reload-plugins
```

Requires `mdsmith` on the `$PATH`.

### mdsmith-autofix

A `PostToolUse` hook that runs `mdsmith fix`
on every `.md` file Claude Code edits. Keeps
generated sections, whitespace, and table
alignment in sync automatically.

```text
/plugin install mdsmith-autofix@mdsmith
/reload-plugins
```

Requires `mdsmith` and `jq` on the `$PATH`.
This plugin is opt-in: if you prefer to run
`/mdsmith-fix` manually, skip it.

### mdsmith-audit

The `markdown-audit` skill. Audits an mdsmith
repository for structural problems the built-in
rules cannot see: hand-maintained indexes,
similar files without a kind, missing
`.mdsmith.yml`, and kinds without a
`path-pattern`.

```text
/plugin install mdsmith-audit@mdsmith
/reload-plugins
```

### mdsmith-dev-lsp

Go and TypeScript LSP servers for mdsmith
contributors. Installs `gopls` and the
TypeScript language server alongside
`mdsmith lsp` for development.

```text
/plugin install mdsmith-dev-lsp@mdsmith
/reload-plugins
```
