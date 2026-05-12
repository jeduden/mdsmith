# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities by opening a [GitHub Security
Advisory](https://github.com/jeduden/mdsmith/security/advisories/new).
Do not file a public issue.

The maintainer aims to acknowledge reports within five business days.

## Supported Versions

Only the latest minor release receives security updates. Pin to a
specific patch version in CI and update via dependabot.

## Supply-Chain Hardening Posture

mdsmith ships through six channels (GitHub Releases, npm, PyPI, mise /
asdf, Visual Studio Marketplace, Open VSX). Every channel is published
from a single GitHub Actions workflow (`.github/workflows/release.yml`)
triggered only by a signed tag push.

The release flow uses:

- **OIDC Trusted Publishing** for npm and PyPI (no long-lived registry
  tokens stored as repo secrets)
- **SLSA build provenance attestations** via
  `actions/attest-build-provenance` for every binary and the `.vsix`
- **Sigstore keyless signatures** (cosign) on the release checksums
- **A scoped `release` GitHub environment** that gates every publishing
  job behind required reviewers
- **Pinned third-party action SHAs** so a tag move on an upstream
  action cannot silently inject malicious behavior
- **`persist-credentials: false`** on every `actions/checkout`
- **`cache: false` / `no-cache: true`** in the release path to
  eliminate GitHub Actions cache poisoning
- **`bun install --frozen-lockfile --ignore-scripts`** for the VS
  Code extension so install-time hooks of dev dependencies cannot
  execute
- **A CI guard** (`npm-lifecycle-guard` in `ci.yml`) that fails any
  PR introducing `preinstall` / `postinstall` / `install` lifecycle
  scripts to the published npm manifests
- **zizmor self-audit** of every workflow in CI, failing on any
  finding

The threat model is in the [supply-chain hardening
note](docs/security/2026-05-12-supply-chain-hardening.md). It walks
through the TanStack / mini-shai-hulud worm class step by step.

## Verifying a Release

Every release artifact carries SLSA provenance and a cosign signature
on the checksum file. To verify:

```bash
# Verify the binary's build provenance
gh attestation verify mdsmith-linux-amd64 -R jeduden/mdsmith

# Verify the checksum file was signed by this exact workflow
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp \
    "^https://github.com/jeduden/mdsmith/.github/workflows/release.yml@" \
  --certificate-oidc-issuer \
    https://token.actions.githubusercontent.com \
  checksums.txt

# Verify the binary against the signed checksum
sha256sum -c checksums.txt --ignore-missing
```

A successful `cosign verify-blob` proves the checksum file was
produced by `release.yml` on this repository at a tag that triggered
the workflow. The matching `sha256sum -c` proves the binary matches
the checksum.
