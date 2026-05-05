// Package release exercises the helper scripts that the release
// workflow runs to publish mdsmith through additional channels
// (npm, PyPI, asdf, mise, the VS Code marketplaces). The package
// has no exported API; its only job is to host the integration
// tests that keep the shell scripts honest.
package release
