---
command: trust
summary: Review the .mdsmith.yml diff since it was last trusted and update the build trust marker on this clone.
---
# `mdsmith trust`

```text
mdsmith trust [--yes] [--config PATH]
```

`mdsmith fix` runs the build pass — the part that executes
`<?build?>` recipes — only when the config is trusted on the current
clone. A config is trusted when the marker file `.mdsmith.yml.trust`
sits beside `.mdsmith.yml` and its bytes are identical. `mdsmith trust`
creates or refreshes that marker.

The command reads the current `.mdsmith.yml`, compares it to the stored
marker, and:

- prints a unified diff of what changed since the config was last
  trusted (the whole config when no marker exists yet),
- prompts for confirmation, and
- on accept, rewrites `.mdsmith.yml.trust` (mode `0600`) with the
  current config bytes.

If the marker already matches the config, the command reports that and
makes no change.

The marker is per-clone state; list it in `.gitignore`. CI runners opt
in with `MDSMITH_TRUST_BUILD=1` instead of a committed marker — see the
[build directive guide](../../guides/directives/build.md#the-trust-gate).

## Flags

| Flag             | Behavior                                            |
| ---------------- | --------------------------------------------------- |
| `-y`, `--yes`    | Skip the confirmation prompt and trust immediately  |
| `-c`, `--config` | Override the config file path (default: discovered) |

## Examples

```bash
# Review the diff and trust interactively.
mdsmith trust

# Trust without a prompt (e.g. a controlled provisioning script).
mdsmith trust --yes
```

## Exit codes

| Code | Meaning                                                 |
| ---- | ------------------------------------------------------- |
| 0    | Config trusted, or already trusted (no change)          |
| 1    | The confirmation prompt was declined                    |
| 2    | The config could not be read or the marker write failed |
