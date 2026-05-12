---
title: Release Pipeline
summary: >-
  How tag pushes publish mdsmith to npm, PyPI, the
  Visual Studio Marketplace, Open VSX, and GitHub
  Releases — the workflow structure, the OIDC trusted
  publishers it relies on, the `release` environment
  that gates every publishing job, and the
  supply-chain hardening features baked into the
  pipeline.
---
# Release Pipeline

A single GitHub Actions workflow publishes mdsmith to
six channels: `.github/workflows/release.yml`. Every
release-time secret travels as a short-lived OIDC
token. The two long-lived publisher PATs that remain
(`VSCE_PAT`, `OVSX_PAT`) are gated by the `release`
GitHub environment.

## Triggering a Release

A maintainer tags the commit and pushes:

```bash
git tag v0.13.0
git push origin v0.13.0
```

`release.yml` is wired to `on: push: tags: ["v*"]`,
so the tag push starts the pipeline. The workflow
also sets `concurrency: { group: release }`, which is
tag-agnostic: any second tag push (same tag or
different) waits for the in-flight run to finish
before starting. The publish window cannot be raced.

No other trigger publishes anything. There is no
`workflow_dispatch`, no `pull_request_target`, no
`workflow_run` chained from CI.

## Channels and Jobs

```text
build (matrix: 5 platforms)
  ├─ vscode       → mdsmith-X.Y.Z.vsix    → Marketplace + Open VSX
  ├─ npm          → @mdsmith/cli + 5 platform packages → npm registry
  ├─ pypi         → mdsmith wheels         → PyPI
  └─ release      → checksums + provenance → GitHub Releases
       └─ smoke-test (matrix: npm / pip / mise)
```

| Job          | Publishes to                     | Credential                         |
|--------------|----------------------------------|------------------------------------|
| `build`      | upload-artifact (intra-workflow) | none                               |
| `vscode`     | Marketplace, Open VSX, artifact  | `VSCE_PAT`, `OVSX_PAT` (env-gated) |
| `npm`        | npm registry (6 packages)        | OIDC Trusted Publishing            |
| `pypi`       | PyPI                             | OIDC Trusted Publishing            |
| `release`    | GitHub Releases, Sigstore, OIDC  | `GITHUB_TOKEN`, OIDC (env-gated)   |
| `smoke-test` | none — verifies channels resolve | none                               |

Every job that holds a credential is also gated by
`if: github.repository == 'jeduden/mdsmith'` and runs
in the `release` GitHub environment.

## OIDC Trusted Publishing

npm and PyPI accept the workflow's GitHub OIDC token
in place of a long-lived API token. The OIDC token
embeds claims describing the workflow run; the
registry-side Trusted Publisher config decides which
claim combinations are allowed to publish.

**npm Trusted Publisher** — configure at
`https://www.npmjs.com/package/<name>/access` for each
of the six packages (`@mdsmith/cli`,
`@mdsmith/linux-x64`, `@mdsmith/linux-arm64`,
`@mdsmith/darwin-x64`, `@mdsmith/darwin-arm64`,
`@mdsmith/win32-x64`):

| Field       | Value                                                 |
|-------------|-------------------------------------------------------|
| Repository  | `jeduden/mdsmith`                                     |
| Workflow    | `release.yml`                                         |
| Environment | `release`                                             |
| Ref         | `refs/tags/v*` (the most specific pattern npm allows) |

Packages that do not exist yet are configured as
[pending publishers](https://docs.npmjs.com/trusted-publishers)
at `https://www.npmjs.com/settings/<user>/trusted-publishers`
before the first publish.

**PyPI Trusted Publisher** — configure at
<https://pypi.org/manage/project/mdsmith/settings/publishing/>:

| Field             | Value             |
|-------------------|-------------------|
| GitHub Repository | `jeduden/mdsmith` |
| Workflow filename | `release.yml`     |
| Environment       | `release`         |

**npm 11.5+ requirement.** Older npm CLIs fall back
to token auth and the registry returns 404 instead
of 401 (so package existence is not leaked) when no
token is present. `release.yml` pins Node 24 (which
ships npm 11.x) and adds an explicit version guard
that fails the job if `npm --version` is below 11.5.

**2FA-required publishing.** Configure at the org
level once for each package:

```bash
npm access 2fa-required @mdsmith/cli
# repeat for each platform package
```

Trusted Publishing satisfies the 2FA requirement
because the OIDC token already proves the workflow's
identity.

## The `release` GitHub Environment

Every job that holds a publishing credential — `npm`,
`pypi`, `vscode`, `release` — declares
`environment: release`. The `environment` claim then
appears in the OIDC token and is pinned by the npm
and PyPI Trusted Publisher configs above.

Configure the environment at
<https://github.com/jeduden/mdsmith/settings/environments>:

| Setting                      | Value                           |
|------------------------------|---------------------------------|
| Required reviewers           | jeduden                         |
| Wait timer                   | 5 minutes (cancellation window) |
| Deployment branches and tags | Selected — protected tags: `v*` |

Without these protections the `environment` claim is
purely decorative. The Trusted Publishers reject any
workflow run whose claim set does not include
`environment=release`, so the env must exist before
the first release.

## Long-Lived Publisher Tokens

Two secrets used in the release pipeline are still
long-lived PATs. The `release` environment gates them
so a workflow run outside an approved release cannot
read them.

| Secret     | Used by        | Scope                | Rotation |
|------------|----------------|----------------------|----------|
| `VSCE_PAT` | `vsce publish` | Marketplace > Manage | Annually |
| `OVSX_PAT` | `ovsx publish` | Open VSX publisher   | Annually |

Record each rotation date in `CLAUDE.md`.
`MERGE_QUEUE_TOKEN`, used by `merge-queue.yml`
outside the release path, stays a plain repo secret.

## Supply-Chain Hardening

The release pipeline applies the following controls.
Each item is enforced by the workflow file, the npm
manifest, or a CI guard — convention alone is not
enough.

- **OIDC Trusted Publishing** for npm and PyPI — no
  long-lived registry tokens stored as repo secrets.
- **SLSA build provenance attestations** via
  `actions/attest-build-provenance` for every binary
  and the `.vsix`. Each attestation ties the file's
  SHA-256 to this workflow run and the commit it was
  built from.
- **Sigstore keyless signatures** (cosign 3.x with
  `--bundle`) on `checksums.txt`. The signing
  certificate's subject is the `release.yml` workflow
  on this repository at the tag that triggered it.
- **`release` GitHub environment** gating every
  publishing job behind required-reviewer rules.
- **`if: github.repository == 'jeduden/mdsmith'`** on
  every publishing job, so a fork-cloned release
  workflow cannot reach the publish steps.
- **`concurrency: { group: release }`** (tag-agnostic)
  to serialize every release run, so no two publish
  jobs can race the registry at the same time.
- **Pinned third-party action SHAs** so a tag move
  on an upstream action cannot silently inject
  behavior.
- **`persist-credentials: false`** on every
  `actions/checkout` (except the demo asset push,
  which only writes to the orphan `assets` branch).
- **`cache: false` / `no-cache: true`** in the
  release path. The GitHub Actions tool cache is the
  fork-base trust boundary the TanStack worm
  exploited; the release path does not restore from
  it.
- **`bun install --frozen-lockfile --ignore-scripts`**
  for every VS Code extension install. Lifecycle
  hooks from dev dependencies are the install-time
  code-execution path the shai-hulud / TanStack worm
  class relies on.
- **`npm-lifecycle-guard` CI job** rejects any PR
  adding an install-, uninstall-, prepare-, pack-,
  or publish-time lifecycle hook to the published
  manifests. The `banned=` line in
  `.github/workflows/ci.yml` is the canonical list.
- **zizmor self-audit** of every workflow in CI,
  failing the job on any finding.
- **No `pull_request_target`, no `workflow_run`, no
  `actions/github-script`, no `github:owner/repo#sha`
  git URL dependencies** — the four upstream surfaces
  most often abused by the shai-hulud worm class do
  not exist in this repo.
- **CODEOWNERS** requires owner review on every file
  under `.github/workflows/` and on this document.

The [hardening note](../security/2026-05-12-supply-chain-hardening.md)
walks through the above against the TanStack /
mini-shai-hulud attack chain.

## Operational Checklist

Run this list once when bootstrapping a fresh clone
or after rotating credentials. Each item below is a
configuration the workflow assumes is already in
place.

1. [ ] Create the `release` environment at
   <https://github.com/jeduden/mdsmith/settings/environments>
   with the values in the table above.
2. [ ] Add the npm Trusted Publisher for each of the
   six packages with `environment=release` and
   `ref=refs/tags/v*`.
3. [ ] Add the PyPI Trusted Publisher with the same
   environment scope.
4. [ ] Enable `2fa-required` on every npm package.
5. [ ] Store `VSCE_PAT` and `OVSX_PAT` as repo
   secrets scoped to the `release` environment.
6. [ ] Enable branch protection on `main` requiring
   CODEOWNERS review for the paths in
   `.github/CODEOWNERS`.
7. [ ] Enable required signed tags for `v*` so an
   unsigned tag cannot trigger a release.

## Verifying a Released Artifact

End-user verification is documented in the
[installation guide](../guides/install.md#github-release-direct-download).
It covers `cosign verify-blob`, `gh attestation
verify`, and `sha256sum -c`. Each step resolves
through the workflow's GitHub OIDC identity. A
forged binary or rewritten checksums file fails
verification unless the attacker also controls
`release.yml` on this repository.
