# mdsmith

<?include
file: ../docs/brand/messaging.md
extract: tagline.text
?>
Write content; mdsmith keeps your Markdown neat and consistent — fast enough to stay out of your way. Auto-fix on save, instant navigation, cross-file integrity, and generated sections that keep a single source of truth in sync across files and pipelines.
<?/include?>

This is the PyPI distribution of the Go binary published at
<https://github.com/jeduden/mdsmith>.

## Install

```bash
pip install mdsmith
# or, without a permanent install:
uvx mdsmith --help
pipx install mdsmith
```

The package ships as one platform-tagged wheel per supported host
(linux x86_64, linux aarch64, macOS x86_64, macOS arm64, Windows
amd64). Each wheel bundles the prebuilt Go binary under
`mdsmith/_bin/`. The `mdsmith` console script execs that binary, so
no compilation or network call runs at install time.

## Versioning

Every PyPI release matches a `vX.Y.Z` git tag in the upstream repo.
`mdsmith version` reports the same value on every distribution
channel (npm, PyPI, asdf, mise, the GitHub release, the VS Code
marketplaces).

## Other channels

See [docs/guides/install.md](https://github.com/jeduden/mdsmith/blob/main/docs/guides/install.md)
for the full list (npm / npx, asdf, mise, direct download, VS Code
Marketplace, Open VSX).
