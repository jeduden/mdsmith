package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeduden/mdsmith/internal/archetypes"
	"github.com/jeduden/mdsmith/internal/config"
)

const archetypesUsage = `Usage: mdsmith archetypes <subcommand> [args]

Subcommands:
  init [dir]         Scaffold an archetype directory with an example.
                     Defaults to ./archetypes. Safe to re-run: existing
                     files are preserved.

  list               Print every archetype discovered under the
                     configured roots, one per line as
                     "<name>\t<path>".

  show <name>        Print the raw schema source of the named archetype
                     to stdout.

  path <name>        Print the resolved filesystem path of the named
                     archetype.

Archetype roots are configured under 'archetypes.roots' in
.mdsmith.yml. When unset, the single root './archetypes' is used.
`

// runArchetypes dispatches the archetypes subcommand.
func runArchetypes(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, archetypesUsage)
		return 0
	}

	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(os.Stderr, archetypesUsage)
		return 0
	case "init":
		return runArchetypesInit(args[1:])
	case "list":
		return runArchetypesList(args[1:])
	case "show":
		return runArchetypesShow(args[1:])
	case "path":
		return runArchetypesPath(args[1:])
	default:
		fmt.Fprintf(os.Stderr,
			"mdsmith: archetypes: unknown subcommand %q\n\n%s",
			args[0], archetypesUsage)
		return 2
	}
}

// archetypesResolver builds a resolver rooted at the current working
// directory using configured roots (or the default).
func archetypesResolver(configPath string) (*archetypes.Resolver, *config.Config, string, error) {
	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		return nil, nil, "", err
	}
	rootDir := rootDirFromConfig(cfgPath)
	roots := cfg.Archetypes.Roots
	return &archetypes.Resolver{
		Roots:   roots,
		RootDir: rootDir,
	}, cfg, rootDir, nil
}

const archetypesInitExample = `---
title: 'string & != ""'
"status?": '"draft" | "review" | "approved"'
---
# ?

## Overview

## Details

## ...
`

const archetypesInitReadme = `# Archetypes

This directory holds required-structure schemas for common
document types used in this repository. Each ` + "`<name>.md`" + `
file is an archetype referenced by name via:

` + "```yaml" + `
rules:
  required-structure:
    archetype: <name>
` + "```" + `

Use ` + "`mdsmith archetypes list`" + ` to see what is available and
` + "`mdsmith archetypes show <name>`" + ` to print one.
`

// runArchetypesInit scaffolds an archetype directory with an example
// schema and a README. Does not mutate the user's .mdsmith.yml.
func runArchetypesInit(args []string) int {
	dir := "archetypes"
	switch len(args) {
	case 0:
	case 1:
		if args[0] == "--help" || args[0] == "-h" {
			fmt.Fprintln(os.Stderr, "Usage: mdsmith archetypes init [dir]")
			return 0
		}
		dir = args[0]
	default:
		fmt.Fprintln(os.Stderr, "mdsmith: archetypes init takes at most one argument")
		return 2
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	examplePath := filepath.Join(dir, "example.md")
	readmePath := filepath.Join(dir, "README.md")

	wroteExample, err := writeIfAbsent(examplePath, archetypesInitExample)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	wroteReadme, err := writeIfAbsent(readmePath, archetypesInitReadme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	fmt.Fprintf(os.Stderr, "mdsmith: archetypes directory ready at %s\n", dir)
	if wroteExample {
		fmt.Fprintf(os.Stderr, "  created %s\n", examplePath)
	} else {
		fmt.Fprintf(os.Stderr, "  kept    %s\n", examplePath)
	}
	if wroteReadme {
		fmt.Fprintf(os.Stderr, "  created %s\n", readmePath)
	} else {
		fmt.Fprintf(os.Stderr, "  kept    %s\n", readmePath)
	}
	fmt.Fprintln(os.Stderr, "\nAdd this to .mdsmith.yml to register the directory:")
	fmt.Fprintf(os.Stderr, "\narchetypes:\n  roots:\n    - %s\n", dir)
	return 0
}

// writeIfAbsent writes content to path only when path does not exist.
// Returns true when the file was created.
func writeIfAbsent(path, content string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// runArchetypesList prints each archetype discovered under the
// configured roots.
func runArchetypesList(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith archetypes list")
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "mdsmith: archetypes list takes no arguments")
		return 2
	}
	resolver, _, _, err := archetypesResolver("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	entries := resolver.List()
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr,
			"mdsmith: no archetypes found under roots %v\n",
			resolver.EffectiveRoots())
		return 1
	}
	for _, e := range entries {
		path := e.Path
		if resolver.RootDir != "" {
			path = filepath.Join(resolver.RootDir, e.Path)
		}
		if _, err := fmt.Fprintf(os.Stdout, "%s\t%s\n", e.Name, path); err != nil {
			return 2
		}
	}
	return 0
}

// runArchetypesShow prints the raw archetype schema source.
func runArchetypesShow(args []string) int {
	name, code := singleNameArg("show", args)
	if code >= 0 {
		return code
	}
	resolver, _, _, err := archetypesResolver("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	body, err := resolver.Content(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if _, err := os.Stdout.Write(body); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// runArchetypesPath prints the resolved filesystem path.
func runArchetypesPath(args []string) int {
	name, code := singleNameArg("path", args)
	if code >= 0 {
		return code
	}
	resolver, _, _, err := archetypesResolver("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	p, err := resolver.AbsPath(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if _, err := fmt.Fprintln(os.Stdout, p); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// singleNameArg extracts a single positional name from args. Returns
// a negative exit code on success, otherwise the exit code to return.
func singleNameArg(verb string, args []string) (string, int) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith archetypes %s <name>\n", verb)
		return "", 0
	}
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr,
			"mdsmith: archetypes %s requires exactly one archetype name\n", verb)
		return "", 2
	}
	return args[0], -1
}
