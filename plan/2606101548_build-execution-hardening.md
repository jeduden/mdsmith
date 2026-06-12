---
id: 2606101548
title: Build execution hardening
status: "✅"
summary: >-
  Layer security on top of plan 2606101546's basic
  builder execution. Trust gate so a freshly
  cloned repo cannot run recipes silently.
  Hermetic env (allowlisted PATH and env
  pass-through). Atomic-write hardening
  (random-suffix staging, refuse if the
  `.mdsmith/build-staging/` root is a
  symlink or world-writable, symlink-safe
  rename). Output
  post-conditions: every declared output
  must exist; no undeclared write may slip
  out. Process-group kill on timeout.
model: opus
depends-on: [2606101546]
---
# Build execution hardening

## Goal

Make the build pass safe on an untrusted
repo. Plan 2606101546 wires recipes through
`os/exec`; this plan adds the defenses that
stop a hostile recipe or config from
escaping its declared outputs, leaking
child processes, or writing elsewhere.

## Context

The threat model treats `.mdsmith.yml` and
`<?build?>` directives as untrusted. Plan
2606101546 assumes trusted input; this plan
closes the gap for freshly cloned repos.

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
- `mdsmith trust` (a new subcommand) diffs
  the current `.mdsmith.yml` against the
  stored trust contents and overwrites the
  marker on confirmation.
- `mdsmith fix --no-build` is the only
  override: it skips the build pass without
  touching the trust marker.

The trust file is per-clone (in
`.gitignore`). CI environments opt in via
`MDSMITH_TRUST_BUILD=1` instead of a file —
they are presumed sandboxed.

A run with neither `<?build?>` directives
nor `build.hooks` (plan 104) never consults
the gate — nothing would execute. Plan
104's MDS040-clean gate runs after this
one. The gate covers `.mdsmith.yml` because
it is the only file that can declare
`build:`; if config ever spreads across
files, the gate must widen with it.

### Hermetic execution environment

Each recipe is invoked with:

- `Cmd.Env` is exactly `PATH` plus the
  `build.exec.env-pass-through` names:
  `PATH` from `build.exec.path` (default
  `/usr/bin:/bin` on Unix), pass-through
  default `[HOME, LANG, LC_ALL]`. Nothing
  else leaks in.
- `Cmd.Dir` set to the per-recipe staging
  dir (see "Atomic write hardening" below).
- A new process group via `Setpgid` on
  Unix; on Windows `CREATE_NEW_PROCESS_GROUP`
  plus a Job Object for the kill path.
- Standard streams attached per plan 2606101547;
  this plan is process control only.

On `--build-timeout` expiry, mdsmith sends
SIGTERM to the process group, waits up to
5 s, then sends SIGKILL. Windows has no
SIGTERM: there mdsmith sends
`CTRL_BREAK_EVENT` to the group, waits 5 s,
then kills the group via its Job Object. A
recipe that spawns daemons cannot leave
orphans behind.

### Atomic write hardening

Plan 2606101546's basic atomic write is replaced
by:

1. mdsmith `Lstat`s `.mdsmith/build-staging/`.
   If absent, it creates the dir and
   `Chmod`s to `0o700` (umask filters
   `MkdirAll`'s mode). A symlink or
   non-directory is refused.
2. mdsmith refuses if
   `.mdsmith/build-staging/` is group- or
   world-writable on Unix (`0o022` mask);
   the user fixes the permissions.
3. mdsmith creates the per-recipe staging
   dir via `os.MkdirTemp` (random suffix)
   under `.mdsmith/build-staging/`. With
   steps 1-2 this stops a hostile parent
   from planting a symlink at the temp name.
4. Each declared output path maps to a
   file inside the staging dir; mdsmith
   pre-creates parent dirs there. Only
   `{outputs}` is substituted with staging
   paths; named params are never rewritten.
5. After post-condition checks (below),
   mdsmith renames each staged file out,
   `Lstat`ing the destination first: a
   symlink fails the build. The replace is
   atomic per file — POSIX `rename(2)`, or
   `MoveFileEx(MOVEFILE_REPLACE_EXISTING)`
   on Windows (what `os.Rename` wraps), for
   same-volume paths. Multi-output rename
   is *not* transactional: if rename N+1
   fails after N, mdsmith logs the partial
   state, removes staging, and exits FAIL;
   the next `fix` reruns the recipe (no
   cache write happened).
6. On any pre-rename failure the staging
   dir is removed; outputs stay untouched.

### Output post-conditions

After a recipe exits 0, two checks run
before the rename phase (Bazel issue
14543 lesson):

- **All declared outputs exist** in the
  staging dir. A missing one is a build
  failure ("recipe exited 0 but did not
  produce X").
- **No undeclared write** landed in the
  project tree. mdsmith snapshots the
  output-paths' parent dirs (file list, size,
  mtime, mode, sha256 of contents) before
  the recipe and diffs after. Content hashing
  catches edits that preserve size and mtime;
  mode catches `chmod`. Any added, removed,
  or modified file outside declared
  `outputs:` is a build failure.

Two known limits. It only covers the parent
dirs of declared outputs; full-tree scans
are too expensive, so writes into an
unrelated subtree are missed. Symlinks are
snapshotted via `Lstat` metadata plus
`os.Readlink`, never followed. A snapshot
scope above 2 000 directory entries is a
build error naming the oversized dir —
point outputs at a narrower directory.

PATH allowlisting and `Cmd.Dir` are for
build determinism, not filesystem
confinement: a recipe can still write via
absolute paths or `../`, and writes outside
the snapshot scope are undetected. Real
confinement needs a sandbox.

### Config schema additions

```yaml
build:
  exec:
    path: "/usr/bin:/bin:/opt/pandoc/bin"
    env-pass-through: [HOME, LANG, LC_ALL, SOURCE_DATE_EPOCH]
```

Both keys are optional. `env-pass-through`
*replaces* the default list (no append) —
re-list the defaults you still want; the
example adds `SOURCE_DATE_EPOCH` for
reproducible builds. MDS040 rejects empty
pass-through names or names containing `=`.

## Tasks

1. Implement the trust gate in
   `internal/build/trust.go`: read
   `.mdsmith.yml.trust`, compare its bytes
   to the current `.mdsmith.yml`, honor
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
   process group + Job Object (Windows),
   group kill on timeout.
5. Replace plan 2606101546's basic atomic
   write with the hardened version:
   staging-root checks, `os.MkdirTemp`
   per-recipe dir, per-destination `Lstat`
   symlink refusal, `os.Rename` replace.
   Document the multi-output
   partial-failure semantics (cleanup;
   next `fix` reruns the recipe).
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
  - `mdsmith fix --no-build` skips the gate.
  - A write outside declared `outputs:` is
    a build failure; the file is left in
    place and named in the warning.
  - Recipe exiting 0 without producing a
    declared output is a build failure.
  - A group- or world-writable staging
    root is refused at start.
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
   `MDSMITH_TRUST_BUILD=1`. Export it in
   `demo.tape`'s hidden setup so the build
   demo (plan 2606101546) keeps running.
   Note that the default `build.exec.path`
   omits mdsmith's own install dir; a
   self-invoking recipe adds it.

## Acceptance Criteria

- [x] Build pass refuses to run when
      `.mdsmith.yml.trust` is missing or
      stale (and `MDSMITH_TRUST_BUILD=1`
      is not set); lint-fix still runs
- [x] `mdsmith trust` shows the config
      diff and updates the trust marker
      on confirmation
- [x] `mdsmith fix --no-build` skips the
      trust check and the build pass
      together
- [x] Recipe writing outside `outputs:`
      is a build failure; the undeclared
      file is named in the diagnostic
- [x] Recipe exiting 0 without producing
      every declared output is a build
      failure
- [x] Atomic write uses `os.MkdirTemp` with
      a random suffix under
      `.mdsmith/build-staging/`; that
      staging root is refused if it is a
      symlink, not a directory, or group-
      or world-writable
- [x] Rename phase `Lstat`s each output
      destination, refuses to replace a
      symlink, then uses `os.Rename`;
      multi-output partial failure cleans
      up the staging dir and exits with
      FAIL (next `fix` reruns the recipe)
- [x] Recipe is invoked with `Cmd.Env`
      restricted to the allowlist and
      `Cmd.Dir` set to the per-recipe
      staging dir
- [x] A snapshot scope above 2 000
      directory entries is a build error
      naming the oversized dir
- [x] Recipe runs in its own process
      group; timeout fires SIGTERM, then
      SIGKILL after 5 s
- [x] `build.exec.path` and
      `build.exec.env-pass-through`
      parse, validate, and take effect
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run`
      reports no issues
