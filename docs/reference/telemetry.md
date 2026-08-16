---
title: Telemetry and runtime network access
summary: >-
  mdsmith collects no telemetry, no usage analytics, no error
  reports, and no identifiers. In the default configuration,
  the CLI and LSP make zero outbound network calls. MDS072
  external-link-check is the one opt-in exception.
---
# Telemetry and runtime network access

mdsmith does not phone home. In the default configuration, the
CLI and the LSP server make zero outbound network calls at
runtime. No telemetry, no analytics, no error reports, no
anonymous identifiers, no update checks.

## What runs offline

- `mdsmith check` walks the workspace and reads files. No network
  by default. (`external-link-check` is the one opt-in exception;
  see below.)
- `mdsmith fix` rewrites files in place. No network by default.
  (Same opt-in exception as `mdsmith check`.) Its build pass runs
  user-declared recipes (see below).
- `mdsmith lsp` speaks LSP over stdio to the parent editor. No
  network by default. (Same opt-in exception as `mdsmith check`.)
- `mdsmith deps`, `mdsmith rename`, `mdsmith metrics`, `mdsmith query`,
  and every other subcommand stay local.

In the default configuration, a locked-down or air-gapped CI
runner can run `mdsmith check .` with no outbound access.

## Install-time network access

Network access only happens when the user installs the binary:

- `go install …/mdsmith@latest` fetches the module from the Go
  proxy.
- `npm install -g @mdsmith/cli` downloads the npm tarball and the
  matching platform sub-package.
- `pip install mdsmith` downloads the wheel from PyPI.
- The VS Code Marketplace or Open VSX downloads the `.vsix`.

None of these channels run a `postinstall` script that calls home.
After install, the binary is a static Go executable. In the
default configuration, running it makes no outbound network calls.

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
itself never calls an LLM at runtime. In the default
configuration, it contacts no external service either. The plugin
spawns `mdsmith lsp` as a local subprocess and feeds its JSON-RPC
output to the editor. Diagnostics, fixes, and navigation all come
from the local Go binary.

## What about the "size and readability limits"?

The five rules grouped under
[Size and readability limits](../features/size-and-readability.md)
(`MDS022`, `MDS023`, `MDS024`, `MDS028`, `MDS037`) are pure
heuristics. They run inside the Go binary. No model inference, no
remote scoring, no embedding lookups.

## Opt-in network access: MDS072 external-link-check

One opt-in rule makes runtime network calls:
`external-link-check` (MDS072). The rule is off by default.
When enabled, it issues an HTTP HEAD request (with a GET fallback
on 405 Method Not Allowed) to each http or https URL found in
documents it lints. Results are cached per URL for the run, so
the same URL across many files costs one request.

**SSRF risk.** The rule probes URLs taken from document content.
When enabled on untrusted content, a hostile document can include
URLs targeting internal hosts — loopback addresses, RFC 1918
private ranges, link-local addresses, or cloud-metadata endpoints.
External URLs can also redirect inward: the rule follows up to
10 redirects, so a pattern in `links.external-skip` that matches
the initial URL does not block a redirect to an internal host.
Use `links.external-skip` to exclude internal address patterns,
or keep the rule disabled when linting untrusted workspaces.

With `external-link-check` disabled (the default), no outbound
traffic is generated and the air-gapped CI claim above holds.

## How to verify

Run `mdsmith check .` under a network-monitoring tool of your
choice (`strace -e trace=network`, `tcpdump`, your firewall) and
inspect the output. In the default configuration, no outbound
traffic appears.
