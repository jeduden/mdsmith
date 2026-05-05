# mdsmith

Fast, auto-fixing Markdown linter and formatter for docs, READMEs,
and AI-generated content. This is the npm distribution of the Go
binary published at
<https://github.com/jeduden/mdsmith>.

## Install

```bash
npm install -g mdsmith
# or, without a global install:
npx mdsmith --help
```

The package ships a small Node.js shim (`bin/mdsmith.js`) that locates
the prebuilt binary from one of these platform sub-packages and execs
it:

- `@mdsmith/linux-x64`
- `@mdsmith/linux-arm64`
- `@mdsmith/darwin-x64`
- `@mdsmith/darwin-arm64`
- `@mdsmith/win32-x64`

npm installs only the sub-package matching `process.platform` and
`process.arch`. There is no postinstall network call, so the package
works in offline / air-gapped CI.

## Versioning

Every npm release matches a `vX.Y.Z` git tag in the upstream repo.
`mdsmith version` reports the same value on every distribution
channel (npm, PyPI, asdf, mise, the GitHub release, the VS Code
marketplaces).

## Other channels

See [docs/guides/install.md](https://github.com/jeduden/mdsmith/blob/main/docs/guides/install.md)
for the full list (PyPI / pip / uvx, asdf, mise, direct download,
VS Code Marketplace, Open VSX).
