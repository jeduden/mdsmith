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

The `security-audit-sarif` workflow uploads
every audit on the most recent date to GitHub
code scanning. The findings then show in the
Security tab, beside CodeQL and zizmor.

<?catalog
glob:
  - "docs/security/*/report.md"
sort: -date
header: |
  | Date | Review | Scope |
  |------|--------|-------|
row: "| {date} | [{title}]({filename}) | {scope} |"
?>
| Date       | Review                                                                                                                 | Scope                                                                                                                                                                                                                                                                                                                                                                                                           |
| ---------- | ---------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-08-14 | [mdsmith post-audit diff review — 2026-08-14](docs/security/2026-08-14-post-audit-diff-review/report.md)               | diff since the 2026-07-24 post-audit diff review (base fe9bbae) — 91 changed files: performance optimizations across the include/crossfile/foreignregion/lint/lsp/schema/goldmark surfaces, the merge-driver -merge -> merge=text exclude change, the new MDS060 occurrence rule, the reworked MDS073 slidev slide-structure rule, and corpus-collection size limiting |
| 2026-08-07 | [mdsmith post-audit diff review — 2026-08-07](docs/security/2026-08-07-post-audit-diff-review/report.md)               | diff since the 2026-07-24 post-audit diff review — the new MDS060 occurrence rule, the vendored goldmark html_block tag-lowering rewrite, the slidev MDS073 structural expansion, the corpus byte-limit cap, the ColumnOfOffset negative-offset clamp, the merge-driver/git-hook -merge to merge=text change, and a batch of high-performance-go.md perf refactors |
| 2026-07-31 | [mdsmith post-audit diff review — 2026-07-31](docs/security/2026-07-31-post-audit-diff-review/report.md)               | diff since the 2026-07-24 post-audit diff review — the new MDS060 occurrence rule, MDS073 slidev-structure follow-up, a large high-performance-go.md perf refactor sweep (struct-field reordering, byte-scan gating, memoization), the corpus byte-limit/TOCTOU hardening, the merge-driver -merge -> merge=text fix, the ColumnOfOffset negative-offset clamp, and the goldmark html_block allocation refactor |
| 2026-07-24 | [mdsmith post-audit diff review — 2026-07-24](docs/security/2026-07-24-post-audit-diff-review/report.md)               | diff since the 2026-07-17 post-audit diff review — the new MDS072 external-link-check network-probing rule, the MDS073 slidev slide-structure rule, foreign managed-region protection for fix, the merge-driver -merge scoping fix, and the init.go split                                                                                                                                                       |
| 2026-07-17 | [mdsmith post-audit diff review — 2026-07-17](docs/security/2026-07-17-post-audit-diff-review/report.md)               | diff since the 2026-07-10 post-audit diff review — SARIF 2.1.0 output, apm-input-token placeholder, the init --add pack surface, the okf starter, and the perf/schema churn                                                                                                                                                                                                                                     |
| 2026-07-10 | [mdsmith post-audit diff review — 2026-07-10](docs/security/2026-07-10-post-audit-diff-review/report.md)               | diff since the 2026-07-03 post-audit diff review — word-list file loading, the F001/F002 fixes, directive-engine workspace containment, CLI/engine parser, CI/supply chain                                                                                                                                                                                                                                      |
| 2026-07-03 | [mdsmith post-audit diff review — 2026-07-03](docs/security/2026-07-03-post-audit-diff-review/report.md)               | diff since the 2026-06-19 full-repo audit — directive engine, CLI/engine core, LSP, VS Code extension, CI/supply chain                                                                                                                                                                                                                                                                                          |
| 2026-06-19 | [mdsmith full-repo security audit — 2026-06-19](docs/security/2026-06-19-full-repo-audit/report.md)                    | full repo — all seven threat-model surfaces                                                                                                                                                                                                                                                                                                                                                                     |
| 2026-06-19 | [LSP server and VS Code extension security audit](docs/security/2026-06-19-lsp-vscode-audit/report.md)                 | LSP server and VS Code extension                                                                                                                                                                                                                                                                                                                                                                                |
| 2026-06-12 | [mdsmith security audit — 2026-06-12](docs/security/2026-06-12-full-repo-audit/report.md)                              | full repo — all seven threat-model surfaces                                                                                                                                                                                                                                                                                                                                                                     |
| 2026-06-12 | [Git integration and LSP server audit](docs/security/2026-06-12-git-lsp-audit/report.md)                               | Git integration and LSP server                                                                                                                                                                                                                                                                                                                                                                                  |
| 2026-06-09 | [mdsmith security audit — 2026-06-09](docs/security/2026-06-09-full-repo-audit/report.md)                              | full repo — all surfaces                                                                                                                                                                                                                                                                                                                                                                                        |
| 2026-05-12 | [Supply-Chain Hardening — mini-shai-hulud / TanStack Class](docs/security/2026-05-12-supply-chain-hardening/report.md) | npm, PyPI, VS Code Marketplace, and Open VSX publishing surface; GitHub Actions CI/CD; lockfile and lifecycle-script handling.                                                                                                                                                                                                                                                                                  |
| 2026-04-05 | [Adversarial Markdown Input](docs/security/2026-04-05-adversarial-markdown/report.md)                                  | Adversarial markdown input causing unintended side effects on the host machine                                                                                                                                                                                                                                                                                                                                  |
<?/catalog?>
