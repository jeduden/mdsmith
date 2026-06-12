---
title: Release Pipeline
summary: >-
  How a maintainer-dispatched workflow run publishes
  mdsmith to npm, PyPI, the Visual Studio Marketplace,
  Open VSX, a Flatpak bundle, and GitHub Releases — the
  workflow structure, the OIDC trusted publishers it
  relies on, the `release` environment that gates every
  publishing job, the separate website deploy, and the
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
glob: ["release-channels/*.md", "!release-channels/proto.md"]
where: 'mechanism: "push"'
sort: title
header: |
  | Channel | Release page | Credential |
  |---------|--------------|------------|
row: "| [{title}]({filename}) | <{channelurl}> | {credential} |"
?>
| Channel                                                                    | Release page                                                          | Credential              |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------- | ----------------------- |
| [Flatpak](release-channels/flatpak.md)                                     | <https://github.com/jeduden/mdsmith/releases>                         | GITHUB_TOKEN + OIDC     |
| [GitHub Releases](release-channels/github-releases.md)                     | <https://github.com/jeduden/mdsmith/releases>                         | GITHUB_TOKEN + OIDC     |
| [npm](release-channels/npm.md)                                             | <https://www.npmjs.com/package/@mdsmith/cli>                          | OIDC Trusted Publishing |
| [Obsidian](release-channels/obsidian.md)                                   | <https://github.com/jeduden/mdsmith/releases>                         | GITHUB_TOKEN + OIDC     |
| [Open VSX](release-channels/open-vsx.md)                                   | <https://open-vsx.org/extension/jeduden/mdsmith>                      | OVSX_PAT                |
| [PyPI](release-channels/pypi.md)                                           | <https://pypi.org/project/mdsmith/>                                   | OIDC Trusted Publishing |
| [Visual Studio Marketplace](release-channels/visual-studio-marketplace.md) | <https://marketplace.visualstudio.com/items?itemName=jeduden.mdsmith> | VSCE_PAT                |
<?/catalog?>

## Triggering a Release

A maintainer opens the **Release** workflow in the
Actions tab, clicks **Run workflow**, enters the
version (e.g. `v0.13.0`), and confirms. The run pauses
once, on the `gate` job's `release-approval` reviewer
rule, so that one approval releases every channel.

`release.yml` triggers solely on `workflow_dispatch`
with a required `version` input. It does not trigger
on `push`, tags, the `release` event,
`pull_request_target`, or `workflow_run`. The Releases
UI cannot drive it. A draft release never creates a
tag. The pipeline owns the release object, so an
externally created release would collide.

The `release` job creates the tag itself: `tag_name` and
`target_commitish` on the `action-gh-release` step
point it at the dispatched commit (the branch chosen
in the form, normally `main`). A `preflight` job
validates the `version` input first. It is read
through an env var, never interpolated into a shell.
So a typo fails fast and nothing publishes.

The `release` job uploads every asset to a **draft**
release first. It then publishes the draft as a
separate final step. That step runs
`mdsmith-release publish-release`. Uploading to a
published release is rejected once immutable releases
are on. So the publish must be the last action. The
result is an immutable release.

GitHub writes the release notes. The
`softprops/action-gh-release` step sets
`generate_release_notes: true`. The notes list merged
PRs and commits since the previous tag.

`concurrency: { group: release, cancel-in-progress:
false }` serializes release runs. Two publish jobs
never mint OIDC tokens against the same registry at
once. The flag lets an in-flight publish finish.
Cancelling mid-publish would desync the platform
packages from the root.

## Job Topology

`build` feeds binaries to `npm`, `pypi`, `vscode`, `flatpak`, and `release`.
`vscode` chains off `build`, running `mdsmith-release build-npm` so the
`.vsix` bundles every platform. `flatpak` chains off `build` too, handing
`release` a `.flatpak` bundle of the x86_64 binary to attach to the draft.
`release` runs `smoke-test` against the fresh `npm`, `pypi`, `mise`,
`asdf`, and `go install` channels (plus best-effort `mise-registry`).
The `gate` job chains off `build` and is the lone reviewer-gated job
(environment `release-approval`); every credential-bearing job lists it
in `needs:`, so one approval releases the whole pipeline. Each such job
carries the repository guard and runs in the `release` environment.

Each `smoke-test` entry installs the just-published version as a user
would, asserting `mdsmith version` reports the tag. A broken channel
(v0.40.0: `go install`) fails the release run, not a user.

The website deploy is a **separate workflow**,
`.github/workflows/pages.yml`. It builds
[mdsmith.dev](https://mdsmith.dev/) and deploys to GitHub
Pages on every push to `main` under `docs/**` or
`website/**` (a docs change ships the site with no tool
release), and on manual `workflow_dispatch`.

A tool release ships it too. Its `pages` job gates on
`release` and on `benchmark-publish`. That job hands the
deploy a fresh `benchmark-numbers` artifact, baked into the
site's performance page. So each release shows its own
figures, not the stale committed ones. A failed measurement
blocks the deploy; a slowdown only trips the separate
`bench-regression-gate`.

`pages.yml` runs in the `github-pages` environment and calls
`build-website --no-fix ./docs ./website/content/docs` to
snapshot `docs/` into the Hugo tree. `--no-fix` because
`docs/` is lint-clean on main; the release-flow benchmark
bake is the one exception. It stamps the version from the
caller input or the latest `v*` tag.

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

| Field       | Value                          |
| ----------- | ------------------------------ |
| Repository  | `jeduden/mdsmith`              |
| Workflow    | `release.yml`                  |
| Environment | `release`                      |
| Ref         | `refs/heads/<selected-branch>` |

The OIDC `ref` claim follows the selected branch —
normally `refs/heads/main`. The `release` environment
restricts deploys to `main`. Publishing jobs cannot
acquire credentials from another branch. Set the
Trusted Publisher `ref` row to match.

Packages that do not exist yet are configured as
[pending publishers](https://docs.npmjs.com/trusted-publishers)
at `https://www.npmjs.com/settings/<user>/trusted-publishers`
before the first publish.

**PyPI Trusted Publisher** — configure at
<https://pypi.org/manage/project/mdsmith/settings/publishing/>:

| Field             | Value             |
| ----------------- | ----------------- |
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

## The `release` and `release-approval` Environments

Two environments split the work: scoping credentials,
and gating on a human.

**`release`** scopes the credentials. Every job that
publishes or nudges a channel — `npm`, `pypi`,
`vscode`, `release`, `winget-submit`, and the
Homebrew/Scoop dispatch jobs — declares
`environment: release`. That routes the env-scoped
secrets (`VSCE_PAT`, `OVSX_PAT`, `WINGET_PR_TOKEN`, and
the two dispatch tokens) to the jobs that read them. It
also puts the `environment=release` claim in the OIDC
token, which the npm and PyPI Trusted Publisher configs
pin. This environment carries no reviewer — only the
main-only branch restriction.

**`release-approval`** is the lone human gate. The
`gate` job is its only user, and every credential job
lists `gate` in `needs:`. So no secret is readable and
no OIDC token is minted before the one approval. That
split collapses the prompts into one. The old flow
asked three times: after `build`, then for `release`,
then for `winget-submit`.

Configure both at
<https://github.com/jeduden/mdsmith/settings/environments>:

| `release-approval`           | Value                               |
| ---------------------------- | ----------------------------------- |
| Required reviewers           | jeduden                             |
| Wait timer                   | 5 minutes (cancellation window)     |
| Deployment branches and tags | Selected — protected branch: `main` |

| `release`                    | Value                                                                |
| ---------------------------- | -------------------------------------------------------------------- |
| Required reviewers           | none — moved to `release-approval`                                   |
| Deployment branches and tags | Selected — protected branch: `main`                                  |
| Secrets                      | `VSCE_PAT`, `OVSX_PAT`, `WINGET_PR_TOKEN`, + Homebrew/Scoop dispatch |

Without these protections the `environment` claim is
purely decorative. The Trusted Publishers reject any
run whose claim set omits `environment=release`. So
both environments must exist before the first release.

## How One Approval Unlocks Every Secret

```text
reviewer approval
  │
  ▼
gate  ── environment: release-approval   (the only required reviewer)
  │
  │  every credential job declares  needs: [gate]
  ▼
npm · pypi · vscode · release · winget-submit ·
notify-homebrew-tap · notify-scoop-bucket
  │  each declares  environment: release
  │  → entering the environment provisions its secrets
  ▼
VSCE_PAT · OVSX_PAT · WINGET_PR_TOKEN ·
HOMEBREW_TAP_DISPATCH_TOKEN · SCOOP_BUCKET_DISPATCH_TOKEN
+ the environment=release OIDC claim  (npm / PyPI Trusted Publishing)
```

`release` holds the secrets but no reviewer;
`release-approval` holds the reviewer but no secrets.
The `needs: [gate]` edge is the only thing between a
credential job and a secret, so the one approval gates
them all. The `release-gate-guard` CI job keeps this a
checked guarantee rather than a convention. It scans
every workflow under `.github/workflows/`. A job that
targets `environment: release` fails the build when it
drops the `gate` edge, hides the environment behind an
expression, or carries an approval-bypassing `if:`
(`always()`, `failure()`, `cancelled()`). Any other
workflow that targets either environment — or hides one
behind an expression — fails the build too.

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
  on this repository for the dispatched run.
- **Single reviewer-gated `gate` job** (environment
  `release-approval`) in every credential job's
  `needs:`, so one approval gates every publish.
- **`release-gate-guard` CI job** (plus a Go test)
  scans every workflow and fails the build on any
  `environment: release` job that omits
  `needs: [gate]`, on approval-bypassing `if:`
  conditions, and on any sibling workflow that targets
  the release environments. Runs
  `mdsmith-release check-release-gates`, then
  `check-release-smoke` (smoke coverage, no soft-skips).
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

The [hardening note](../security/2026-05-12-supply-chain-hardening/report.md)
walks through the above against the TanStack /
mini-shai-hulud attack chain.

## Operational Checklist

Run this list once when bootstrapping a fresh clone
or after rotating credentials. Each item below is a
configuration the workflow assumes is already in
place.

1. [ ] Create the `release` and `release-approval`
   environments at
   <https://github.com/jeduden/mdsmith/settings/environments>
   with the values in the tables above. The reviewer
   goes on `release-approval` only.
2. [ ] Add the npm Trusted Publisher to every
   published package with `environment=release` and
   `ref=refs/heads/main` (see the npm Trusted
   Publisher section above for the package list).
3. [ ] Add the PyPI Trusted Publisher with the same
   environment scope.
4. [ ] Enable `2fa-required` on every npm package.
5. [ ] Store `VSCE_PAT`, `OVSX_PAT`, `WINGET_PR_TOKEN`,
   `HOMEBREW_TAP_DISPATCH_TOKEN`, and
   `SCOOP_BUCKET_DISPATCH_TOKEN` as secrets on the
   `release` environment. Delete any old plain-repo
   copies of the same names: a job reads the
   environment copy, so a stale repo-level token would
   linger unrotated — and an empty environment copy
   makes the dispatch jobs skip silently.
6. [ ] Enable branch protection on `main` requiring
   CODEOWNERS review for the paths in
   `.github/CODEOWNERS`.
7. [ ] Keep `release-approval`'s required reviewer
   set. With `gate` in every credential job's `needs:`,
   it is the only release gate, and the workflow (not a
   maintainer) creates the tag.

## Verifying a Released Artifact

End-user verification is documented in the
[installation guide](../guides/install.md#github-release-direct-download).
It covers `cosign verify-blob`, `gh attestation
verify`, and `sha256sum -c`. Each step resolves
through the workflow's GitHub OIDC identity. A
forged binary or rewritten checksums file fails
verification unless the attacker also controls
`release.yml` on this repository.
