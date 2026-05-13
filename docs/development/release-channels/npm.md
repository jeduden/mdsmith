---
title: npm
summary: >-
  Six packages total: root `@mdsmith/cli` plus one
  platform-specific subpackage per supported host.
  All six publish via OIDC Trusted Publishing.
registry: registry.npmjs.org
credential: OIDC Trusted Publishing
job: npm
---
# npm

The npm channel publishes six packages: the root
and one platform-specific subpackage per supported
host:

- `@mdsmith/cli` — root, contains the shim
- `@mdsmith/linux-x64`
- `@mdsmith/linux-arm64`
- `@mdsmith/darwin-x64`
- `@mdsmith/darwin-arm64`
- `@mdsmith/win32-x64`

The root package's `bin/mdsmith.js` shim resolves
the matching subpackage at runtime via
`require.resolve`. There is no postinstall hook, so
`npm install` runs in offline / air-gapped CI
without network calls.

The `npm` job in `release.yml` publishes the
platform packages first, then the root. The order
matters: the root advertises each platform as an
`optionalDependency`, and would otherwise reference
a package npm cannot find. Both steps run
`npm publish --provenance` to stamp the tarballs
with SLSA build attestations.

Auth is OIDC Trusted Publishing. See the
`OIDC Trusted Publishing` section in
[`release.md`](../release.md) for the npmjs.com
configuration each of the six packages needs.
