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
  abuse, runtime memory extraction of publish tokens). Adds defense
  in depth: concurrency group, repository-and-environment guards on
  publishing jobs, `--ignore-scripts` on every `bun install`, a CI
  guard that fails any future PR introducing npm lifecycle hooks,
  and SECURITY.md.
---
# Security Review: mini-shai-hulud / TanStack Supply-Chain Class

---

## Background

On 2026-05-11 between 19:20 and 19:26 UTC an attacker published 84 malicious versions across 42 `@tanstack/*` npm packages. The same worm family also hit `@mistralai/*`, `@uipath/*`, `@squawk/*`, and others. The Aikido write-up (["Mini Shai-Hulud Is Back: npm Worm Hits over 160 Packages, including Mistral and Tanstack"](https://www.aikido.dev/blog/mini-shai-hulud-is-back-tanstack-compromised)) and the TanStack postmortem (linked from [tanstack/router#7383](https://github.com/TanStack/router/issues/7383)) describe a three-stage chain:

1. **`pull_request_target` "Pwn Request"** — a fork PR triggered a workflow that checked out and executed the fork's code with access to base-repo secrets.
2. **GitHub Actions cache poisoning across the fork↔base trust boundary** — the malicious fork wrote a poisoned pnpm store into the shared cache; the legitimate release workflow restored that cache.
3. **Runtime OIDC token extraction** — the malware located the `Runner.Worker` process, read `/proc/<pid>/maps` and `/proc/<pid>/mem`, and pulled the GitHub Actions OIDC token from memory. Because TanStack's npm Trusted Publisher was scoped at the repository level (not pinned to a specific protected branch / ref / workflow file / environment), the stolen token minted a valid short-lived npm publish credential, and the worm published [validly-attested SLSA Build Level 3 provenance for malicious packages](https://github.com/advisories/GHSA-g7cv-rxg3-hmpx).

A parallel vector (used by other mini-shai-hulud waves) is the classic `preinstall` / `postinstall` lifecycle script that scans the install host for npm tokens, GitHub PATs, `~/.npmrc`, `~/.gitconfig`, SSH keys, and cloud credentials, then republishes infected versions of every package the stolen npm token can write.

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

Reviewing the exact TanStack attack chain step by step:

1. **No `pull_request_target` trigger anywhere** — `grep -rn 'pull_request_target' .github/` returns no hits. Fork PRs cannot execute privileged workflows.
2. **No `workflow_run` trigger** — eliminates the secondary chained-workflow vector other shai-hulud waves used.
3. **No `actions/github-script` step** — no surface for inline JavaScript that runs with the `GITHUB_TOKEN` of a fork PR.
4. **No `github:owner/repo#sha` git URL dependencies** — `grep -rn '"github:' npm/ editors/` is empty, so the fork-network orphan-commit specifier vector cannot reach mdsmith's installable surface.
5. **GitHub Actions tool cache disabled in the release workflow** — `setup-go` is invoked with `cache: false` and `setup-bun` with `no-cache: true`. The release path cannot restore a poisoned cache.
6. **No `preinstall` / `postinstall` / `install` lifecycle hooks** in any published manifest — the npm shim deliberately resolves the platform binary at runtime, not at install time. The `npm-lifecycle-guard` CI job now enforces this in CI rather than relying on convention.
7. **npm publishing uses Trusted Publishing (OIDC)** — no long-lived `NODE_AUTH_TOKEN` for a worm to harvest from `~/.npmrc` or env vars.
8. **PyPI publishing uses Trusted Publishing (OIDC)** — same property.
9. **All third-party GitHub Actions are pinned to commit SHAs** — a tag move on `softprops/action-gh-release`, `actions/checkout`, `pypa/gh-action-pypi-publish`, or `codecov/codecov-action` cannot silently pull a malicious version.
10. **zizmor runs in CI and fails the job on any finding** — the same scanner that flagged the TanStack cache-poisoning surface.

## Defense-in-Depth Hardening Applied 2026-05-12

The above leaves three residual concerns that this change addresses:

### 1. OIDC token scope was workflow-file-only, not environment-bound

The npm Trusted Publisher was configured to require `repo=jeduden/mdsmith` and `workflow=release.yml`. That is strictly better than the TanStack baseline, but still lets any successful `release.yml` run mint a publish token. Adding `environment: release` to every job that holds `id-token: write` (or a long-lived publisher PAT) introduces an `environment` claim in the OIDC token. The maintainer should:

1. Open the [environments settings page](https://github.com/jeduden/mdsmith/settings/environments)
2. Create an environment named `release`
3. Add **required reviewers** (jeduden) and a **wait timer** (e.g. 5 minutes)
4. Restrict deployment branches to **tags matching `v*`**
5. Open the npm Trusted Publisher config for each of the six packages (`@mdsmith/cli` plus five platform subpackages) at `https://www.npmjs.com/package/<name>/access` and add `environment: release` to the constraint set
6. Repeat for PyPI at the [project publishing settings](https://pypi.org/manage/project/mdsmith/settings/publishing/) (set the Environment field to `release`)

With the environment claim required at the npm / PyPI side, an attacker who somehow runs `release.yml` outside the `release` environment (e.g., via a future workflow misconfiguration) cannot mint a valid publish token.

### 2. Concurrent tag pushes could race the publish window

A second `git push --tags` while a release is in flight previously ran the publish jobs in parallel against the same registry record. The new `concurrency: { group: release-${{ github.ref }}, cancel-in-progress: false }` at the workflow level serializes runs — the first publish completes; the second waits and then no-ops because the registry already has the version.

### 3. Lifecycle scripts of dev-time dependencies could run during CI

The VS Code extension's dev dependency tree (`@vscode/vsce`, `typescript`, `@types/*`, `vscode-languageclient`) is pure-JS today, but a future lockfile update or registry compromise could introduce a `postinstall` hook. Every `bun install --frozen-lockfile` invocation now also passes `--ignore-scripts`, neutralizing the install-time code execution path the shai-hulud worm relies on. If a future dep genuinely needs a lifecycle script, the requirement should be documented here before the flag is relaxed for that step.

### 4. CI guard against future manifest tampering

`ci.yml`'s new `npm-lifecycle-guard` job rejects any PR that adds `preinstall` / `install` / `postinstall` / `prepare` / `uninstall` hooks to `npm/mdsmith/package.json` or `editors/vscode/package.json`. The guard is intentionally noisy: changing it requires updating this security note.

## Residual Risk

- **Long-lived `VSCE_PAT` / `OVSX_PAT` / `MERGE_QUEUE_TOKEN`** are still long-lived PATs. The `release` environment gates them behind reviewers; rotate annually (calendar reminders live in `CLAUDE.md`).
- **`codecov-action` with `id-token: write` runs on PRs from same-repo branches.** The OIDC token's audience is `codecov`, so it cannot be replayed against npm or PyPI. The upload step is `if`-gated to skip fork PRs.
- **Lockfiles are not verified against a content-addressed registry.** `bun audit signatures` would catch a registry-side tampering; future work.

## Operational Checklist for the Maintainer

The hardening above is half code, half configuration. After merging the workflow changes, the maintainer should complete:

1. [ ] Create the `release` GitHub environment with required reviewers
2. [ ] Add `environment: release` constraint to each npm Trusted Publisher
3. [ ] Add `environment: release` constraint to the PyPI Trusted Publisher
4. [ ] Confirm npm Trusted Publishers also scope on ref pattern `refs/tags/v*` (not just workflow filename)
5. [ ] Confirm 2FA-required publishing is enabled at the npm org level (`npm access 2fa-required @mdsmith/cli` and each platform package)
6. [ ] Rotate `VSCE_PAT` and `OVSX_PAT` (annual; record date in `CLAUDE.md`)
7. [ ] Enable branch protection on `main` requiring CODEOWNERS review for `.github/workflows/**` and any `package.json`

## References

- [Aikido: Mini Shai-Hulud Is Back](https://www.aikido.dev/blog/mini-shai-hulud-is-back-tanstack-compromised)
- [TanStack postmortem](https://tanstack.com/blog/npm-supply-chain-compromise-postmortem) (linked from [tanstack/router#7383](https://github.com/TanStack/router/issues/7383))
- [GitHub Security Advisory GHSA-g7cv-rxg3-hmpx](https://github.com/advisories/GHSA-g7cv-rxg3-hmpx)
- [npm Trusted Publishers docs](https://docs.npmjs.com/trusted-publishers)
- [PyPI Trusted Publishers docs](https://docs.pypi.org/trusted-publishers/)
- [Datadog Security Labs: Shai-Hulud 2.0 npm worm analysis](https://securitylabs.datadoghq.com/articles/shai-hulud-2.0-npm-worm/)
- [Snyk: NPM Security Best Practices After the 2025 Shai-Hulud Attack](https://snyk.io/articles/npm-security-best-practices-shai-hulud-attack/)
