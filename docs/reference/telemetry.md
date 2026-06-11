---
title: Telemetry and runtime network access
summary: >-
  mdsmith collects no telemetry, no usage analytics, no error
  reports, and no identifiers. The CLI and the LSP server make
  no outbound network calls at runtime.
---
# Telemetry and runtime network access

mdsmith does not phone home. The CLI and the LSP server make zero
outbound network calls during normal operation. No telemetry, no
analytics, no error reports, no anonymous identifiers, no update
checks.

## What runs offline

- `mdsmith check` walks the workspace and reads files. No network.
- `mdsmith fix` rewrites files in place. No network from mdsmith
  itself. Its build pass runs user-declared recipes (see below).
- `mdsmith lsp` speaks LSP over stdio to the parent editor. No
  network.
- `mdsmith deps`, `mdsmith rename`, `mdsmith metrics`, `mdsmith query`,
  and every other subcommand stay local.

A locked-down or air-gapped CI runner can run `mdsmith check .`
with no outbound access and the run completes normally.

## Install-time network access

Network access only happens when the user installs the binary:

- `go install …/mdsmith@latest` fetches the module from the Go
  proxy.
- `npm install -g @mdsmith/cli` downloads the npm tarball and the
  matching platform sub-package.
- `pip install mdsmith` downloads the wheel from PyPI.
- The VS Code Marketplace or Open VSX downloads the `.vsix`.

None of these channels run a `postinstall` script that calls home.
After install, the binary is a static Go executable; running it
makes no network calls.

The [install guide](../guides/install.md#github-release-direct-download)
covers the GitHub-release direct-download path for air-gapped hosts.

## What about `mdsmith fix` build recipes?

The `mdsmith fix` build pass dispatches each `<?build?>` directive to
a recipe you declare in `build.recipes`. A recipe is your own
command, run via `os/exec` with an explicit argv and no shell. What
that command does — including whether it makes a network call — is
under your control, not mdsmith's. mdsmith executes the recipe; it
adds no network access of its own. Pass `--no-build` to skip the
build pass entirely, and `--build-dry-run` to enumerate the targets
without running any recipe. `mdsmith check` never runs a recipe.

## What about the Claude Code plugin?

The Claude Code plugin is an optional editor surface. mdsmith
itself never calls an LLM or any external service at runtime. The
plugin spawns `mdsmith lsp` as a local subprocess and feeds its
JSON-RPC output to the editor. The diagnostics, fixes, and
navigation all come from the local Go binary.

## What about the "size and readability limits"?

The five rules grouped under
[Size and readability limits](../features/size-and-readability.md)
(`MDS022`, `MDS023`, `MDS024`, `MDS028`, `MDS037`) are pure
heuristics. They run inside the Go binary. No model inference, no
remote scoring, no embedding lookups.

## How to verify

Run `mdsmith check .` under a network-monitoring tool of your
choice (`strace -e trace=network`, `tcpdump`, your firewall) and
inspect the output. No outbound traffic appears.
