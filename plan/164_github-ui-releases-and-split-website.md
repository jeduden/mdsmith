---
id: 164
title: 'GitHub-UI-triggered releases and a split website deploy'
status: "✅"
summary: >-
  Trigger tool releases from the Actions
  "Run workflow" UI (workflow_dispatch +
  one release-environment approval) instead
  of a manually pushed tag, and split the
  mdsmith.dev deploy into its own workflow
  so docs-only changes ship the site
  without a tool release.
model: ""
depends-on: []
---
# GitHub-UI-triggered releases and a split website deploy

## Goal

Let a maintainer cut a tool release from
the GitHub Actions UI with one approval.
Keep the draft-first immutable release
flow. Deploy the website independently of
the tool. A docs change then ships the
site but not the binaries.

## Context

`release.yml` triggered only on a pushed
`v*` tag. The GitHub Releases UI could not
drive it: a draft release never creates a
tag, so no `push` event fired. The pipeline
also owns the release object — it creates a
draft, uploads every asset, then flips it
to published — so an externally created
release would collide with that flow.

Decision (user): keep immutability and the
OIDC hardening; switch the trigger to
`workflow_dispatch` with a `version` input,
gated by the existing `release`
environment. That is one UI action plus one
environment approval. The release job now
creates the tag itself
(`tag_name` + `target_commitish`).

Asymmetry the user asked for:

- A tool release also deploys the website.
- A docs/website change deploys only the
  website — no tool release, no approval,
  no version bump.

So the website deploy moves out of
`release.yml`. It lives in its own
`pages.yml`, a reusable workflow. It runs
on `push` to `main` under `docs/**` and
`website/**`. `release.yml` also calls it,
so a tool release still ships the site.

Security note: `workflow_dispatch` changes
the OIDC `ref` claim. It goes from
`refs/tags/v*` to `refs/heads/<branch>`.
So the npm/PyPI Trusted Publisher pin
changes. So does the `release` environment
deployment rule. The primary pin still
holds: `environment=release` plus the
required reviewer. `release.md` documents
this.

## Tasks

1. `release.yml`: replace the `push: tags`
   trigger with `workflow_dispatch` (a
   required `version` input). Add a
   `preflight` job that validates the input
   shape (read via env, not template
   expansion) and carries the
   `&release_repo_trigger_ok` anchor;
   `build` and `vscode` depend on it.
2. `release.yml` release job: set
   `tag_name` and `target_commitish` on the
   `action-gh-release` step so the tag is
   created by the workflow.
3. Add `.github/workflows/pages.yml`: a
   reusable + `push`-on-`main`
   (`docs/**`, `website/**`) workflow that
   builds and deploys mdsmith.dev. It
   resolves the site version from the
   caller input or the latest `v*` tag.
4. `release.yml`: drop the `pages-deploy`
   job; add a `pages` job that calls
   `pages.yml` with the release version so
   a tool release still deploys the site.
5. Docs: update `release.md` (triggering,
   topology, Trusted Publisher `Ref`,
   environment table, operational
   checklist), `release-tooling.md`
   (subcommand invokers), and
   `github-releases.md`.
6. Run `mdsmith fix` so the CLAUDE.md and
   docs catalogs regenerate after the
   `release.md` summary change.

## Acceptance Criteria

- [x] `release.yml` triggers only on
  `workflow_dispatch` with a `version`
  input; no `push:`/tag trigger remains.
- [x] A malformed `version` fails
  `preflight` before any publishing job
  runs.
- [x] The release job creates the tag and
  the immutable draft-first publish flow is
  unchanged.
- [x] `pages.yml` deploys the site on a
  docs-only push to `main` with no tool
  release and no `release`-environment
  approval.
- [x] A `workflow_dispatch` release also
  deploys the site via the `pages` caller
  job.
- [x] Docs reflect the new trigger, the
  OIDC `ref` change, and the split website
  deploy.
- [x] `mdsmith check .` passes.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports
  no issues.
