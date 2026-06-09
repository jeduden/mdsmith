# @mdsmith/cli

<?include
file: ../../docs/brand/messaging.md
extract: tagline.text
?>
Write content; mdsmith keeps your Markdown neat and consistent — fast enough to stay out of your way. Auto-fix on save, instant navigation, cross-file integrity, and generated sections that keep a single source of truth in sync across files and pipelines.
<?/include?>

This is the npm distribution of the Go binary published at
<https://github.com/jeduden/mdsmith>.

The npm root is `@mdsmith/cli` because the unscoped `mdsmith`
name on npm is owned by another project. The installed binary
is still called `mdsmith` (via the package's `bin` field).

## Install

```bash
npm install -g @mdsmith/cli
# or, without a global install:
npx @mdsmith/cli --help
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
