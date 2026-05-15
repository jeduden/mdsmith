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

`.github/workflows/release.yml` publishes mdsmith to
every channel below. Release-time secrets travel as
short-lived OIDC tokens. The remaining long-lived
PATs are gated by the `release` GitHub environment.
Each channel has its own file under
`release-channels/`; the catalog re-renders on
`mdsmith fix`.

<?catalog
glob: "release-channels/*.md"
sort: title
header: |
  | Channel | Credential |
  |---------|------------|
row: "| [{title}]({filename}) | {credential} |"
?>
| Channel                                                                    | Credential              |
|----------------------------------------------------------------------------|-------------------------|
| [GitHub Releases](release-channels/github-releases.md)                     | GITHUB_TOKEN + OIDC     |
| [npm](release-channels/npm.md)                                             | OIDC Trusted Publishing |
| [Open VSX](release-channels/open-vsx.md)                                   | OVSX_PAT                |
| [PyPI](release-channels/pypi.md)                                           | OIDC Trusted Publishing |
| [Visual Studio Marketplace](release-channels/visual-studio-marketplace.md) | VSCE_PAT                |
<?/catalog?>

## Triggering a Release

A maintainer tags the commit and pushes:

```bash
git tag v0.13.0
git push origin v0.13.0
```

A `v*` tag push is the only trigger. `release.yml`
omits `workflow_dispatch`, `pull_request_target`,
and `workflow_run`. Those triggers could mint OIDC
tokens or reach the PATs from a non-tag context.
`concurrency: { group: release, cancel-in-progress: false }`
serializes every run tag-agnostically. A second
push queues. The flag lets the in-flight publish
finish; cancelling mid-publish would desync the
platform packages from the root.

## Job Topology

`build` produces per-platform binaries that fan out
to `vscode`, `npm`, `pypi`, and `release`. The
`release` job runs `smoke-test` against the freshly
published `npm`, `pypi`, and `mise` channels. Every
job that holds a credential is also gated by
`if: github.repository == 'jeduden/mdsmith'` and
runs in the `release` GitHub environment.

`pages-deploy` builds [mdsmith.dev](https://mdsmith.dev/)
from `website/` and deploys it to GitHub Pages. The job
runs independently of the publish chain, so a flaky
registry publish does not block the docs deploy. It
sits in the `github-pages` environment, GitHub's
built-in Pages protection boundary, not the `release`
environment. It gates on the `build` job, so a tag that
fails to compile never deploys docs.

The build step calls `mdsmith-release build-website
--no-fix ./docs ./website/content/docs`. That snapshots
the source-of-truth `docs/` tree into the Hugo content
tree. `--no-fix` is used because `docs/` is already
lint-clean on main; CI must not mutate it.

## OIDC Trusted Publishing

npm and PyPI accept the workflow's GitHub OIDC token
in place of a long-lived API token. The OIDC token
embeds claims describing the workflow run; the
registry-side Trusted Publisher config decides which
claim combinations are allowed to publish.

**npm Trusted Publisher** — configure at
`https://www.npmjs.com/package/<name>/access` for
every package listed in
[the npm channel doc](release-channels/npm.md).
That page is the canonical list. This doc does
not duplicate it.

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

Long-lived publisher tokens (gated by the `release`
environment when applicable, plain repo secret
otherwise) are listed in
[secret-rotations.md](secret-rotations.md). That
page holds the rotation procedure plus a catalog
over per-secret files in
[`secret-rotations/`](secret-rotations/), one file
per tracked secret. A scheduled workflow opens an
issue 30 days before any tracked secret is due.

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
2. [ ] Add the npm Trusted Publisher to every
   published package with `environment=release` and
   `ref=refs/tags/v*` (see the npm Trusted Publisher
   section above for the package list).
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
