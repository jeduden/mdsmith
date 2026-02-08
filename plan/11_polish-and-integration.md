# 11: Polish & Integration

## Goal

Final polish: stdin support, version embedding, end-to-end tests, CI
verification.

## Tasks

1. Stdin support — if stdin is a pipe, read from it (file name = `<stdin>`)
2. `--version` — embed version via `debug.ReadBuildInfo()` (shows module
   version or `(devel)` for local builds)
3. End-to-end tests — run the compiled binary against fixture files, assert
   exit code + stderr output
4. CI — verify `go test ./...` and `golangci-lint run` pass in GitHub Actions

## Acceptance Criteria

### Stdin

- [ ] `echo "# Hello" | tidymark` lints the piped content
- [ ] Stdin diagnostics use `<stdin>` as the file name in output
- [ ] `echo "# Hello" | tidymark --format json` includes `"file": "<stdin>"`
- [ ] `--fix` with stdin input exits 2 with an error (cannot fix stdin in place)

### Version

- [ ] `tidymark --version` prints a version string (module version or `(devel)`)
- [ ] `-v` is a shorthand for `--version`

### End-to-end tests

- [ ] A test runs the compiled `tidymark` binary against a clean `.md` fixture
      and asserts exit code 0
- [ ] A test runs the binary against a fixture with known violations and asserts
      exit code 1 and expected diagnostic text on stderr
- [ ] A test runs the binary with `--format json` and validates the JSON schema
- [ ] A test runs the binary with `--fix` on a copy of a fixture and verifies
      the file is corrected
- [ ] A test runs the binary with `--config` pointing to a custom config and
      verifies rule behavior changes
- [ ] A test runs the binary with no arguments and asserts exit code 0

### CI

- [ ] `go test ./...` passes in GitHub Actions
- [ ] `golangci-lint run` passes in GitHub Actions
- [ ] The CI workflow runs on push to main and on pull requests

### General

- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
