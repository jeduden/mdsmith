---
command: merge-driver
summary: Git merge driver that resolves conflicts inside generated sections.
---
# `mdsmith merge-driver`

Register and run a Git merge driver. It auto-resolves
conflicts inside generated sections — `<?catalog?>`,
`<?include?>`, `<?toc?>`. The driver strips conflict
markers in those blocks and runs `mdsmith fix` to
regenerate them. It exits non-zero if any unresolved
conflict markers remain.

```text
mdsmith merge-driver <subcommand> [args]
```

## Subcommands

### `install [globs...]`

Register the merge driver in `git config` and ensure
`.gitattributes` assigns it. The managed block is derived
from `.mdsmith.yml`. It lists include patterns first
(default `*.md` and `*.markdown`). Then it lists one
`merge=text` exclude line per representable `ignore:`
pattern. Last match wins.

The exclude lines use `merge=text`, git's built-in 3-way
text merge, not `-merge`. Both take the mdsmith driver off
the path, but `-merge` unsets the `merge` attribute — which
declares the file binary and leaves it whole-file-conflicting.
An ignored path is Markdown that mdsmith never lints, so it
carries no generated sections for the driver to reconcile;
`merge=text` keeps ordinary edits merging and avoids that
class of conflict. A `.gitattributes` written by an older
mdsmith that still lists `-merge` is read the same way, so
it does not report as drift until it is regenerated.

Each `ignore:` pattern is scoped to the markdown include
extensions. So `ignore: ["demo/**"]` emits
`demo/**/*.md merge=text` and `demo/**/*.markdown merge=text`.
A bare `demo/** merge=text` is not used. It would also change
git's merge for the source code in that tree.

Some `ignore:` entries cannot be expressed in
`.gitattributes`: `!` negation patterns and patterns with
whitespace. Those are skipped. As a result, the hook's
scope may include files the merge driver isn't registered
for.

Optional positional args replace the default include set
when scoping to a custom pattern (e.g. `docs/**/*.md`).
Custom include globs are not compatible with MDS048
`git-hook-sync` auto-fix, which restores the canonical
default set; do not enable `git-hook-sync` if you rely on
custom includes.

### `ci-install`

Like `install`, but it treats the committed
`.gitattributes` as the source of truth instead of
rewriting it. This is the `npm ci` analogue of `install`'s
`npm install`. It first checks the committed managed block
against the globs derived from `.mdsmith.yml`. It never
writes the file. It exits non-zero when `.gitattributes`
is missing, has no managed block, or has drifted.

Only after that check passes does it register the merge
driver in `git config` and install the pre-merge-commit
hook. Both are git-internal and untracked. A failed
verification exits before either, so a drift error leaves
the repository in place.

Run it in CI and the merge queue. There, `install` would
re-render `.gitattributes` mid-run. A drifted committed
copy would be rewritten, which dirties the worktree. That
aborts the in-progress `git merge`, requeues the PR, and
re-fires the labeled trigger forever. `ci-install` never
writes the tracked file. The merge starts from a clean
tree, and real drift fails one setup step loudly instead
of looping.

`ci-install` takes no glob arguments. It always compares
against the canonical default include set. So it is
incompatible with custom-glob installs, the same as
MDS048.

### `run <base> <ours> <theirs> <pathname>`

Driver entrypoint, invoked by Git. Not normally called by
hand. After `install` or `ci-install`, Git will dispatch to
it whenever merging a file marked `merge=mdsmith`.

The driver writes only the `<ours>` temp file (`%A`). It
regenerates sections by running `mdsmith fix` on the merged
content in memory, anchored at `<pathname>` so neighbour
files resolve as they would for the real file. The worktree
path itself is never read or written. The driver runs while
the parent merge is still in flight, so a worktree write
changes the file's stat data — even when the bytes are
restored exactly. Git then treats the path as locally
modified and aborts the merge ("Your local changes ...
would be overwritten"). Under `git rebase`, a pick that
hits this is rescheduled forever.

## Git config (set by `install` / `ci-install`)

```text
merge.mdsmith.driver = '/abs/path/to/mdsmith' merge-driver run %O %A %B %P
```

The path is the absolute location of the `mdsmith` binary
at install time, shell-quoted so paths with spaces work.

## Examples

```bash
mdsmith merge-driver install
mdsmith merge-driver install 'docs/**/*.md'
mdsmith merge-driver ci-install   # CI / merge queue: verify, don't write
```

After `install`, a merge with conflicts inside a
generated section auto-resolves. After conflicts in
hand-written prose, run `mdsmith fix <file>` once the
rest of the merge is resolved.
