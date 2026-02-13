---
id: 44
title: Rule Docs Command
status: 🔲
---
# Rule Docs Command

## Goal

Embed rule README files into the binary and add a
`tidymark docs` command, so users can read rule docs
offline without visiting the repository.

## Tasks

1. Add an embed directive in a new file
   `internal/ruledocs/embed.go`. Use `//go:embed` to embed
   all files matching `rules/*/README.md`. Export a function
   that returns the embedded filesystem.

2. Write a parser that reads each embedded README and
   extracts front matter fields: rule ID, rule name, and
   one-line description. Store the results in a slice of
   structs for fast lookup.

3. Add lookup functions. Support finding a rule by its ID
   (for example `TM001`) or by its name (for example
   `line-length`). Return the full README content as a
   string. Return an error if the rule is not found.

4. Add the `docs` subcommand to the CLI in
   `cmd/tidymark/main.go`. Register it alongside the
   existing commands.

5. When called with an argument (`tidymark docs TM001` or
   `tidymark docs line-length`), print the matching README
   to stdout. Exit 2 if the rule is not found.

6. When called with no argument (`tidymark docs`), print a
   table of all rules to stdout. Each row shows the rule
   ID, name, and short description. Sort rows by rule ID.

7. Add unit tests for the embed and lookup logic:

  - Lookup by ID returns correct content
  - Lookup by name returns correct content
  - Unknown ID returns an error
  - List mode returns all rules sorted by ID

8. Add integration tests for the CLI:

  - `tidymark docs TM001` prints the TM001 README
  - `tidymark docs line-length` prints the same README
  - `tidymark docs TMXXX` exits 2 with an error message
  - `tidymark docs` lists all rules

9. Update CLI help text and the commands table in
   `CLAUDE.md` to document the new `docs` subcommand.

## Acceptance Criteria

- [ ] Rule READMEs are embedded in the binary at build time
- [ ] `tidymark docs TM001` prints the TM001 README
- [ ] `tidymark docs line-length` prints the TM001 README
- [ ] `tidymark docs` lists all rules with ID and name
- [ ] Unknown rule ID or name exits 2 with an error
- [ ] Output goes to stdout for piping
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
