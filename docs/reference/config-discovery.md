---
weight: 30
summary: >-
  Which file mdsmith reads config from (`.mdsmith.yml` or a
  `pyproject.toml` `[tool.mdsmith]` table), the discovery order and
  precedence, and how a bad config value is reported as a
  `file:line:col` diagnostic.
---
# Config discovery

mdsmith reads its configuration from one file. That file is
either `.mdsmith.yml` or the `[tool.mdsmith]` table of a
`pyproject.toml`. Both sources accept the same keys.

## Discovery order

Without `--config`, mdsmith starts in the working directory.
It walks up toward the filesystem root. Each directory has two
candidates, checked in order.

| Order | File             | Selected when                         |
| ----- | ---------------- | ------------------------------------- |
| 1     | `.mdsmith.yml`   | the file exists                       |
| 2     | `pyproject.toml` | the file has a `[tool.mdsmith]` table |

The first selected file ends the walk. These rules follow:

- In one directory, `.mdsmith.yml` wins over `pyproject.toml`.
- Across directories, the nearest selected file wins.
- A `pyproject.toml` without `[tool.mdsmith]` is not a config
  source. The walk continues past it.
- A directory that contains `.git` is the last directory
  checked.
- With no selected file, the built-in defaults apply.

A `pyproject.toml` that fails to parse is still selected when a
line opens a `[tool.mdsmith` header or sets a `tool.mdsmith.`
dotted key. Loading it then reports the syntax error.

The editor integration (`mdsmith lsp`), the build pass, and the
merge driver's glob set read the same file. The language server
watches `.mdsmith.yml` and `pyproject.toml` and reloads config
when either changes.

## The `--config` flag

`-c`/`--config <path>` skips discovery. A path whose extension
is `.toml` (any case) is read from its `[tool.mdsmith]` table;
any other path is read as YAML. So `--config pyproject.toml`
and `--config ci.toml` both work.

A TOML file without `[tool.mdsmith]` is a config error (exit
`2`).

## The `[tool.mdsmith]` table

The table holds the same keys as `.mdsmith.yml`, written in
TOML. These two configs are equal:

```toml
[tool.mdsmith]
files = ["docs/**/*.md"]
convention = "portable"

[tool.mdsmith.rules]
no-bare-urls = false
line-length = { max = 100 }

[[tool.mdsmith.overrides]]
glob = ["CHANGELOG.md"]
rules = { no-duplicate-headings = false }

[tool.mdsmith.kinds.plan]
path-pattern = "plan/*.md"
```

```yaml
files: ["docs/**/*.md"]
convention: portable
rules:
  no-bare-urls: false
  line-length:
    max: 100
overrides:
  - glob: ["CHANGELOG.md"]
    rules:
      no-duplicate-headings: false
kinds:
  plan:
    path-pattern: "plan/*.md"
```

| TOML form                        | Config shape                       |
| -------------------------------- | ---------------------------------- |
| `rule-name = false`              | rule off                           |
| `[tool.mdsmith.rules.rule-name]` | rule settings mapping              |
| inline table `{ key = value }`   | mapping                            |
| `[[tool.mdsmith.overrides]]`     | one entry of the `overrides:` list |
| offset date-time, local date     | timestamp                          |
| local time                       | string                             |

Other tables in `pyproject.toml` are not read.

Paths in the table resolve against the directory that holds
`pyproject.toml`. The `.mdsmith/` resource directories (kinds,
conventions, schemas, word-lists) are found next to it, as they
are next to `.mdsmith.yml`.

The [build trust](cli/trust.md) marker pins the loaded file, so
a `pyproject.toml` config uses `pyproject.toml.trust`. Any edit
to `pyproject.toml` requires `mdsmith trust` again before
recipes run.

The WebAssembly engine does not read `pyproject.toml`. Its host
passes config as inline YAML text.

### The plural `[tools.mdsmith]` table

The table name is the singular `tool`, as the
`pyproject.toml` standard defines. A `pyproject.toml` with
`[tools.mdsmith]` and no `[tool.mdsmith]` is not a config
source. Discovery skips it and prints one hint:

```text
mdsmith: hint: /repo/pyproject.toml: [tools.mdsmith] is not read; rename the table to [tool.mdsmith]
```

The language server logs the same hint as a warning.

## Config errors

A bad config value is a diagnostic. It points at the line and
column of the offending key. The file is the one that holds the
key. The CLI uses the lint output format. The rule ID is always
`config`. The exit code is `2`.

```text
.mdsmith.yml:5:5 config foreign-regions[0]: end marker must not be empty
pyproject.toml:12:1 config convention: unknown convention "nope" (valid: ...)
```

| Error source                                       | Position reported                                         |
| -------------------------------------------------- | --------------------------------------------------------- |
| bad value in `.mdsmith.yml`                        | the key (or list entry) in `.mdsmith.yml`                 |
| bad value in `[tool.mdsmith]`                      | the key in `pyproject.toml`, inline tables included       |
| bad value in a `.mdsmith/` kind or convention file | the key in that file                                      |
| YAML syntax or type error                          | the line the YAML parser reports, column 1                |
| TOML syntax error                                  | the line and column the TOML parser reports               |
| value with no key in the file                      | the nearest enclosing key, or the `[tool.mdsmith]` header |

An error with no known position, such as an unreadable file,
prints as a plain `mdsmith: <error>` line. The exit code is
`2` in both forms.

The language server publishes the diagnostic on the config file,
so an open `.mdsmith.yml` or `pyproject.toml` shows it inline.
The next clean reload clears it. A `window/logMessage` summary
is sent as well, for when the file is not open.

## See also

- [`mdsmith check`](cli/check.md): the `--config` flag and exit
  codes
- [`mdsmith init`](cli/init.md): write a starting
  `.mdsmith.yml`
- [Config transparency](../features/config-transparency.md):
  how config layers merge
