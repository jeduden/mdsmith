# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities by opening a [GitHub Security
Advisory](https://github.com/jeduden/mdsmith/security/advisories/new).
Do not file a public issue.

The maintainer aims to acknowledge reports within
five business days.

## Supported Versions

Only the latest minor release receives security
updates. Pin to a specific patch version in CI and
update via dependabot.

## Release Pipeline and Supply-Chain Posture

The release pipeline lives in
[`docs/development/release.md`](docs/development/release.md).
It is the single source of truth. It covers the
workflow structure, the OIDC trusted publishers,
the `release` environment that gates publishing
jobs, and the supply-chain hardening features baked
into the pipeline. Each publishing channel has its
own file under
[`docs/development/release-channels/`](docs/development/release-channels/).
The release-pipeline doc enumerates them via a
`<?catalog?>` directive.

## Verifying a Released Artifact

Cosign, `gh attestation verify`, and `sha256sum -c`
commands live in the
[installation guide](docs/guides/install.md#github-release-direct-download).
Every step resolves through the workflow's GitHub
OIDC identity. A forged binary or rewritten
checksums file fails verification unless the
attacker also controls `release.yml` on this
repository.

## Security Audit Log

Point-in-time security reviews live in
[`docs/security/`](docs/security/). Each review is a
directory named `YYYY-MM-DD-<slug>/`. It holds a
`report.md` next to its machine-readable companions:
`findings.json`, `findings.sarif`, and
`inline-annotations.json`. The report records the
scope, the method, the findings, and the follow-up.

<?catalog
glob:
  - "docs/security/*/report.md"
sort: -date
header: |
  | Date | Review | Scope |
  |------|--------|-------|
row: "| {date} | [{title}]({filename}) | {scope} |"
?>
| Date       | Review                                                                                                                 | Scope                                                                                                                          |
| ---------- | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| 2026-06-09 | [mdsmith security audit — 2026-06-09](docs/security/2026-06-09-full-repo-audit/report.md)                              | full repo — all surfaces                                                                                                       |
| 2026-05-12 | [Supply-Chain Hardening — mini-shai-hulud / TanStack Class](docs/security/2026-05-12-supply-chain-hardening/report.md) | npm, PyPI, VS Code Marketplace, and Open VSX publishing surface; GitHub Actions CI/CD; lockfile and lifecycle-script handling. |
| 2026-04-05 | [Adversarial Markdown Input](docs/security/2026-04-05-adversarial-markdown/report.md)                                  | Adversarial markdown input causing unintended side effects on the host machine                                                 |
<?/catalog?>
