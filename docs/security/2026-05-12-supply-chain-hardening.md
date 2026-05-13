---
date: "2026-05-12"
scope: >-
  npm, PyPI, VS Code Marketplace, and Open VSX publishing surface;
  GitHub Actions CI/CD; lockfile and lifecycle-script handling.
method: >-
  Threat-model review against the TanStack / mini-shai-hulud npm
  worm (2026-05-11), followed by gap fixes in release.yml, ci.yml,
  and SECURITY.md.
title: Supply-Chain Hardening — mini-shai-hulud / TanStack Class
summary: >-
  Confirms mdsmith is not vulnerable to the direct TanStack attack
  chain (pull_request_target pwn, fork-network orphan-commit OIDC
  abuse, runtime memory extraction of publish tokens). Adds
  defense-in-depth controls: concurrency group, repository-and-
  environment guards on publishing jobs, `--ignore-scripts` on every
  `bun install`, a CI guard that fails any future PR introducing npm
  lifecycle hooks, and centralizes the release pipeline documentation.
---
# Security Review: mini-shai-hulud / TanStack Supply-Chain Class

The steady-state release pipeline and the full list
of supply-chain controls live in
[`docs/development/release.md`](../development/release.md).
This note is a point-in-time threat model: what the
attack was, which surfaces it touched in mdsmith, and
which gaps the 2026-05-12 commit closed.

---

## Background

On 2026-05-11 between 19:20 and 19:26 UTC an attacker
published 84 malicious versions across 42
`@tanstack/*` npm packages. The same worm family also
hit `@mistralai/*`, `@uipath/*`, `@squawk/*`, and
others. The [Aikido write-up][aikido] and the
[TanStack postmortem][tanstack-post] (linked from
[tanstack/router#7383][router-issue]) describe a
three-stage chain:

1. **`pull_request_target` "Pwn Request"** — a fork
   PR triggered a workflow that checked out and
   executed the fork's code with access to base-repo
   secrets.
2. **GitHub Actions cache poisoning across the
   fork↔base trust boundary** — the malicious fork
   wrote a poisoned pnpm store into the shared
   cache; the legitimate release workflow restored
   that cache.
3. **Runtime OIDC token extraction** — the malware
   located the `Runner.Worker` process, read
   `/proc/<pid>/maps` and `/proc/<pid>/mem`, and
   pulled the GitHub Actions OIDC token from memory.
   Because TanStack's npm Trusted Publisher was
   scoped at the repository level (not pinned to a
   protected branch / ref / workflow file /
   environment), the stolen token minted a valid
   short-lived npm publish credential. The worm
   published [validly-attested SLSA Build Level 3
   provenance for malicious packages][ghsa].

A parallel vector used by other mini-shai-hulud
waves is the classic `preinstall` / `postinstall`
lifecycle script that scans the install host for npm
tokens, GitHub PATs, `~/.npmrc`, `~/.gitconfig`, SSH
keys, and cloud credentials. The worm then
republishes infected versions of every package the
stolen npm token can write.

## Surfaces Reviewed

| Surface                               | Path                            | Verdict                                                                                     |
|---------------------------------------|---------------------------------|---------------------------------------------------------------------------------------------|
| npm root package                      | `npm/mdsmith/`                  | No lifecycle scripts; `files:` allowlist; npm shim uses frozen platform-package map.        |
| npm platform packages                 | built by `mdsmith-release`      | No lifecycle scripts (binary-only).                                                         |
| PyPI wheel                            | `python/`                       | Wheel-only (no sdist); no install-time code.                                                |
| VS Code extension                     | `editors/vscode/`               | `vsce package --no-dependencies` strips deps from the published `.vsix`.                    |
| Claude Code plugin                    | `editors/claude-code/`          | Marketplace metadata only; no executable payload.                                           |
| Release workflow                      | `.github/workflows/release.yml` | OIDC trusted publishing, pinned actions, `cache: false`, `persist-credentials: false`.      |
| CI workflow                           | `.github/workflows/ci.yml`      | `pull_request` only (no `pull_request_target`); zizmor self-audit; codecov OIDC fork-gated. |
| Demo / merge-queue / record workflows | `.github/workflows/`            | No untrusted-fork execution paths.                                                          |

## Why mdsmith Is Not Directly Vulnerable

Reviewing the exact TanStack attack chain step by
step:

1. **No `pull_request_target` trigger anywhere** —
   `grep -rn 'pull_request_target' .github/` returns
   no hits. Fork PRs cannot execute privileged
   workflows.
2. **No `workflow_run` trigger** — eliminates the
   secondary chained-workflow vector other shai-
   hulud waves used.
3. **No `actions/github-script` step** — no surface
   for inline JavaScript that runs with the
   `GITHUB_TOKEN` of a fork PR.
4. **No `github:owner/repo#sha` git URL
   dependencies** — `grep -rn '"github:' npm/
   editors/` is empty, so the fork-network orphan-
   commit specifier vector cannot reach mdsmith's
   installable surface.
5. **GitHub Actions tool cache disabled in the
   release workflow** — `setup-go` is invoked with
   `cache: false` and `setup-bun` with
   `no-cache: true`. The release path cannot restore
   a poisoned cache.
6. **No `preinstall` / `postinstall` / `install`
   lifecycle hooks** in any published manifest. The
   npm shim deliberately resolves the platform
   binary at runtime, not at install time. The
   `npm-lifecycle-guard` CI job now enforces this in
   CI rather than relying on convention.
7. **npm publishing uses Trusted Publishing
   (OIDC)** — no long-lived `NODE_AUTH_TOKEN` for a
   worm to harvest from `~/.npmrc` or env vars.
8. **PyPI publishing uses Trusted Publishing
   (OIDC)** — same property.
9. **All third-party GitHub Actions are pinned to
   commit SHAs** — a tag move on
   `softprops/action-gh-release`, `actions/checkout`,
   `pypa/gh-action-pypi-publish`, or
   `codecov/codecov-action` cannot silently pull a
   malicious version.
10. **zizmor runs in CI and fails the job on any
    finding** — the same scanner that flagged the
    TanStack cache-poisoning surface.

## Defense-in-Depth Hardening Applied 2026-05-12

The above leaves four residual concerns. The
2026-05-12 commit closes them and centralizes the
ongoing posture in
[`docs/development/release.md`](../development/release.md).

### 1. OIDC token scope was workflow-file-only

The npm Trusted Publisher was configured to require
`repo=jeduden/mdsmith` and `workflow=release.yml`.
That is strictly better than the TanStack baseline.
A successful `release.yml` run on any ref could
still mint a publish token. Adding
`environment: release` to every job that holds
`id-token: write` (or a long-lived publisher PAT)
introduces an `environment` claim in the OIDC token.
With the environment claim required at the npm /
PyPI side, an attacker who somehow runs `release.yml`
outside the `release` environment cannot mint a valid
publish token. The
[release pipeline operational checklist](../development/release.md#operational-checklist)
lists the corresponding npmjs.com, pypi.org, and
GitHub UI steps.

### 2. Concurrent tag pushes could race the publish window

A second `git push --tags` while a release was in
flight previously ran the publish jobs in parallel
against the same registry record. The new
`concurrency: { group: release, cancel-in-progress: false }`
at the workflow level serializes every release run
— tag-agnostic, so different tags pushed back-to-back
queue rather than overlap. The first publish
completes; the second runs after.

### 3. Lifecycle scripts of dev-time dependencies could run during CI

The VS Code extension's dev dependency tree
(`@vscode/vsce`, `typescript`, `@types/*`,
`vscode-languageclient`) is pure-JS today. A future
lockfile update or registry compromise could
introduce a `postinstall` hook. Every
`bun install --frozen-lockfile` invocation now also
passes `--ignore-scripts`, neutralizing the install-
time code execution path the shai-hulud worm relies
on. If a future dep genuinely needs a lifecycle
script, the requirement should be documented here
before the flag is relaxed for that step.

### 4. CI guard against future manifest tampering

`ci.yml`'s new `npm-lifecycle-guard` job rejects any
PR that adds an install-, uninstall-, prepare-, pack-,
or publish-time lifecycle hook to
`npm/mdsmith/package.json` or
`editors/vscode/package.json`. The full banned set
lives in the `banned=` line of
`.github/workflows/ci.yml` (15 hooks at time of
writing: `preinstall`, `install`, `postinstall`,
`preuninstall`, `uninstall`, `postuninstall`,
`preprepare`, `prepare`, `postprepare`, `prepublish`,
`prepublishOnly`, `prepack`, `postpack`, `publish`,
`postpublish`). The guard is intentionally noisy:
changing it requires updating this security note.

## Residual Risk

- **Long-lived publisher PATs.** `VSCE_PAT` and
  `OVSX_PAT` are gated by the `release` GitHub
  environment so a workflow run outside an approved
  release cannot read them. `MERGE_QUEUE_TOKEN`,
  consumed by `merge-queue.yml` on PR-label events,
  is intentionally not env-scoped (the merge queue
  runs on every PR-merge cycle, not tag pushes) — its
  blast radius is branch-protection bypass on `main`.
  The scheduled
  `secret-rotation-reminder.yml` workflow opens a
  labelled GitHub issue 30 days before each tracked
  secret's `lastRotated + periodDays` due date, and
  `record-secret-rotation.yml` is the corresponding
  `workflow_dispatch` that records the rotation by
  opening a PR. Per-secret files under
  `docs/development/secret-rotations/` hold the
  rotation procedure; see
  [secret-rotations.md](../development/secret-rotations.md)
  for the catalog and overall workflow.
- **`codecov-action` with `id-token: write` runs on
  PRs from same-repo branches.** The OIDC token's
  audience is `codecov`, so it cannot be replayed
  against npm or PyPI. The upload step is `if`-gated
  to skip fork PRs.
- **Lockfiles are not verified against a content-
  addressed registry.** `bun audit signatures` would
  catch a registry-side tampering; future work.

## References

- Aikido: [Mini Shai-Hulud Is Back][aikido]
- [TanStack postmortem][tanstack-post] linked from
  [tanstack/router#7383][router-issue]
- [GitHub Security Advisory GHSA-g7cv-rxg3-hmpx][ghsa]
- [npm Trusted Publishers docs][npm-tp]
- [PyPI Trusted Publishers docs][pypi-tp]
- [Datadog Security Labs: Shai-Hulud 2.0 npm worm
  analysis][datadog]
- [Snyk: NPM Security Best Practices After the 2025
  Shai-Hulud Attack][snyk]

[aikido]: https://www.aikido.dev/blog/mini-shai-hulud-is-back-tanstack-compromised
[tanstack-post]: https://tanstack.com/blog/npm-supply-chain-compromise-postmortem
[router-issue]: https://github.com/TanStack/router/issues/7383
[ghsa]: https://github.com/advisories/GHSA-g7cv-rxg3-hmpx
[npm-tp]: https://docs.npmjs.com/trusted-publishers
[pypi-tp]: https://docs.pypi.org/trusted-publishers/
[datadog]: https://securitylabs.datadoghq.com/articles/shai-hulud-2.0-npm-worm/
[snyk]: https://snyk.io/articles/npm-security-best-practices-shai-hulud-attack/
