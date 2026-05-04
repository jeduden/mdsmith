---
id: 117
title: Build execution hardening
status: "🔲"
summary: >-
  Layer security on top of plan 115's basic
  builder execution. Trust gate so a freshly
  cloned repo cannot run recipes silently.
  Hermetic env (allowlisted PATH and env
  pass-through). Atomic-write hardening
  (random-suffix staging, world-writable
  parent refusal, symlink-safe rename). Output
  post-conditions: every declared output
  must exist; no undeclared write may slip
  out. Process-group kill on timeout.
model: opus
---
# Build execution hardening

## Goal

Make the build pass safe to run on an
untrusted repo. Plan 115 wires the recipe
through `os/exec`. Plan 117 adds the
defenses that prevent a hostile recipe (or
a hostile config) from escaping its
declared inputs/outputs, leaking child
processes, or writing where it should not.

## Context

The threat model treats both `.mdsmith.yml`
and `<?build?>` directives as untrusted.
Plan 115's wiring works against trusted
input. This plan closes the gap so cloning
a strange repo and running `mdsmith fix`
does not detonate.

## Design

### Trust gate

mdsmith treats `.mdsmith.yml` as untrusted.
A freshly cloned repo may declare recipes
that run arbitrary binaries. The build pass
refuses to run until the user marks the
config as trusted (direnv-style):

- `mdsmith fix` runs the lint-fix pass
  unconditionally.
- The build pass runs only when a sibling
  file `.mdsmith.yml.trust` exists and its
  content is byte-for-byte identical to the
  current `.mdsmith.yml`. Any drift makes
  the build pass exit with a clear "config
  changed since trusted; review and re-trust"
  message.
- `mdsmith trust` (a tiny new subcommand)
  diffs the current `.mdsmith.yml` against
  the stored `.mdsmith.yml.trust` contents
  and overwrites `.mdsmith.yml.trust` with
  the current config on confirmation.
- `mdsmith fix --no-build` is the only
  override: it skips the build pass without
  touching the trust marker.

The trust file is per-clone (in
`.gitignore`). CI environments opt in via
`MDSMITH_TRUST_BUILD=1` instead of a file —
they are presumed sandboxed.

### Hermetic execution environment

Each recipe is invoked with:

- `Cmd.Env` set to a minimal allowlist:
  `PATH=<from build.exec.path>` (default:
  `/usr/bin:/bin` on Unix, system defaults
  on Windows), `HOME`, `LANG`, `LC_ALL`,
  plus any name in
  `build.exec.env-pass-through`.
- `Cmd.Dir` set to the per-recipe staging
  dir (see "Atomic write hardening" below).
- A new process group via `Setpgid` on
  Unix (or `CREATE_NEW_PROCESS_GROUP` on
  Windows).
- Standard streams attached per plan 116;
  this plan is process control only.

On `--build-timeout` expiry, mdsmith
signals the entire process group with
SIGTERM, waits up to 5 s, then sends
SIGKILL. This prevents a recipe that
spawns daemons or workers from leaving
zombies behind.

### Atomic write hardening

Plan 115's basic atomic write is replaced
by:

1. mdsmith creates the staging dir via
   `os.MkdirTemp` with a random suffix under
   `.mdsmith/build-staging/`. Putting the
   staging root under the project root
   prevents a hostile output dir from
   pre-creating a symlink at the temp
   name.
2. mdsmith refuses to proceed if
   `.mdsmith/build-staging/` is world-
   writable on Unix (mode bit `0o002`
   set). The user is asked to fix the
   directory permissions.
3. Each declared output path maps to a
   file inside the staging dir. The recipe
   gets the staging path substituted for
   `{outputs}` and any output-path params.
4. After post-condition checks (below),
   mdsmith renames each staged file to its
   final location. For each destination,
   mdsmith first `Lstat`s the existing path:
   if it is a symlink, the build fails
   ("output path is a symlink; refuse to
   replace"). Otherwise mdsmith uses
   `os.Rename`, which atomically replaces
   the destination on POSIX systems.
   Multi-output rename is *not* transactionally
   atomic across files: per-file rename is
   atomic, but if rename N+1 fails after N
   succeeded, mdsmith logs the partial state
   ("rebuild left in inconsistent state;
   re-run to recover"), removes the staging
   dir, and exits with FAIL. The next
   `mdsmith fix` reruns the recipe (the
   ActionID still mismatches the cache
   because no cache write happened).
5. On any pre-rename failure, the staging
   dir is removed; no declared output is
   touched.

### Output post-conditions

After a recipe exits 0, mdsmith runs two
checks before the rename phase (Bazel
issue 14543 lesson):

- **All declared outputs exist** in the
  staging dir. A missing one is a build
  failure ("recipe exited 0 but did not
  produce X"). Recipe stdout claiming
  success is not enough.
- **No undeclared write** landed in the
  project tree. mdsmith snapshots the
  output-paths' parent dirs (file list +
  size + mtime + mode + sha256 of contents)
  before the recipe and diffs after. Hashing
  catches edits that preserve size and mtime;
  mode catches `chmod`-only changes. Any
  added, removed, or modified file outside
  the declared `outputs:` is a build failure.

The undeclared-write check has two known
limits:

- It only covers the parent dirs of declared
  outputs (full-tree scans would be too
  expensive for large repos). A recipe that
  writes into an unrelated subtree is missed.
- Hashing happens once before and once after
  per file. Symlinks in the snapshot are
  recorded by `Lstat` target, not by following.

Writes outside are constrained by the
hermetic env's allowlisted PATH and the
absence of any `Cmd.Dir` outside the
staging dir.

### Config schema additions

```yaml
build:
  exec:
    path: "/usr/bin:/bin"
    env-pass-through: [HOME, LANG, LC_ALL]
```

Both keys are optional. Defaults are listed
above. MDS040 validates that no
pass-through name is empty or contains `=`.

## Tasks

1. Implement the trust gate in
   `internal/build/trust.go`: read
   `.mdsmith.yml.trust`, compare its bytes
   to the current `.mdsmith.yml`, honour
   `MDSMITH_TRUST_BUILD=1`, and refuse the
   build pass on mismatch.
2. Add `mdsmith trust` subcommand: print
   the diff (using `diff`-style output)
   between `.mdsmith.yml` and the stored
   `.mdsmith.yml.trust` contents, prompt
   for confirmation, overwrite
   `.mdsmith.yml.trust` with the current
   config on accept.
3. Extend `BuildConfig` in
   `internal/config/build.go` with
   `Exec ExecCfg` (path, env-pass-through).
   MDS040 validates entries.
4. Implement hermetic invocation in
   `internal/build/exec.go`: minimal
   `Cmd.Env` from the allowlist, `Cmd.Dir`
   set to staging, `Setpgid` (Unix) or
   process group (Windows), SIGTERM-then-
   SIGKILL on timeout.
5. Replace plan 115's basic atomic write
   with the hardened version: `os.MkdirTemp`
   staging under `.mdsmith/build-staging/`,
   world-writable parent refusal,
   `RENAME_NOREPLACE` (Linux) or symlink-
   checked `os.Rename` (others), full
   rollback on partial rename failure.
6. Implement output post-conditions in
   `internal/build/postcheck.go`: snapshot
   staging dir + output parents pre-recipe,
   diff post-recipe, fail on missing
   declared outputs or undeclared writes.
7. Integration tests:

  - Missing `.mdsmith.yml.trust` blocks
    the build pass; lint-fix still runs.
  - `MDSMITH_TRUST_BUILD=1` is an
    alternate trust source.
  - Editing `.mdsmith.yml` after trust
    invalidates the marker; `mdsmith
    trust` shows the diff and re-trusts.
  - `mdsmith fix --no-build` skips the
    gate.
  - Recipe writing a file outside its
    declared `outputs:` is a build
    failure; the file is left in place
    with a warning that points the user
    to it.
  - Recipe exiting 0 without producing a
    declared output is a build failure.
  - World-writable
    `.mdsmith/build-staging/` parent dir
    is refused at start.
  - A recipe that spawns a child process
    and exceeds `--build-timeout` is
    killed (process group); the child is
    not orphaned.
  - A recipe is invoked with
    `Cmd.Env` containing only the
    allowlisted names.

8. Document the trust gate, hermetic env,
   atomic-write hardening, and output
   post-conditions in
   `docs/guides/directives/build.md`.
   Include CI guidance for
   `MDSMITH_TRUST_BUILD=1`.

## Acceptance Criteria

- [ ] Build pass refuses to run when
      `.mdsmith.yml.trust` is missing or
      stale (and `MDSMITH_TRUST_BUILD=1`
      is not set); lint-fix still runs
- [ ] `mdsmith trust` shows the config
      diff and updates the trust marker
      on confirmation
- [ ] `mdsmith fix --no-build` skips the
      trust check and the build pass
      together
- [ ] Recipe writing outside `outputs:`
      is a build failure; the undeclared
      file is named in the diagnostic
- [ ] Recipe exiting 0 without producing
      every declared output is a build
      failure
- [ ] Atomic write uses `os.MkdirTemp` with
      a random suffix under
      `.mdsmith/build-staging/`; world-
      writable staging parent is refused
- [ ] Rename phase uses
      `RENAME_NOREPLACE` (Linux) or a
      symlink-checked fallback elsewhere;
      partial rename failure rolls back
- [ ] Recipe is invoked with `Cmd.Env`
      restricted to the allowlist and
      `Cmd.Dir` set to the per-recipe
      staging dir
- [ ] Recipe runs in its own process
      group; timeout fires SIGTERM, then
      SIGKILL after 5 s
- [ ] `build.exec.path` and
      `build.exec.env-pass-through`
      parse, validate, and take effect
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run`
      reports no issues
