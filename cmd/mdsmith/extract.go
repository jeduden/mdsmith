package main

import (
	"fmt"
	"os"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/extract"
	"github.com/jeduden/mdsmith/internal/extract/encode"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/requiredstructure"
	"github.com/jeduden/mdsmith/internal/schema"
	flag "github.com/spf13/pflag"
)

// runExtract implements the "extract" subcommand:
//
//	mdsmith extract <kind> --format <fmt> <file>
//
// It projects a schema-conformant file into a data tree whose shape
// mirrors the composed schema. Extraction is gated on a clean
// `check`: a non-conformant file prints the same diagnostics and
// exits non-zero, never emitting partial data.
func runExtract(args []string) int {
	kindName, path, fmtEnum, stop := parseExtractArgs(args)
	if stop >= 0 {
		return stop
	}

	res, cfg, code := resolveFileFromCLI(path)
	if code != 0 {
		return code
	}
	_, cfgPath, _ := loadConfig("")
	if code := validateExtractKind(cfg, res, kindName, path); code != 0 {
		return code
	}

	maxBytes, err := resolveMaxInputBytes(cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if code := gateExtractCheck(cfg, cfgPath, path, maxBytes); code != 0 {
		return code
	}

	f, source, code := loadExtractFile(cfg, cfgPath, path, maxBytes)
	if code != 0 {
		return code
	}
	sch, code := composedSchemaFor(f, res, kindName)
	if code != 0 {
		return code
	}

	var docFM map[string]any
	if frontMatterEnabled(cfg) {
		if prefix, _ := lint.StripFrontMatter(source); len(prefix) > 0 {
			docFM, _ = lint.ParseFrontMatterFields(prefix)
		}
	}

	mt := schema.BuildMatchTree(f, sch, docFM)
	data, diags := extract.Extract(f, sch, mt)
	if len(diags) > 0 {
		formatDiagnostics(diags, "text", false)
		return 1
	}

	out, err := encode.Encode(fmtEnum, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: encoding %s: %v\n", fmtEnum, err)
		return 2
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
		return 2
	}
	return 0
}

// parseExtractArgs parses flags and the two positionals. The final
// return is -1 to continue, or a process exit code to stop on
// (usage error, --help, or a bad --format value).
func parseExtractArgs(
	args []string,
) (kindName, path string, fmtEnum encode.Format, stop int) {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	var format string
	fs.StringVarP(&format, "format", "f", "json",
		"Output format: json, yaml, msgpack")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith extract <kind> --format <fmt> <file>\n\n"+
				"Emit a kind-conformant file as a data tree whose nesting\n"+
				"mirrors the schema hierarchy.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: extract"); code >= 0 {
			return "", "", "", code
		}
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr,
			"mdsmith: extract requires <kind> and <file>")
		return "", "", "", 2
	}
	f, err := encode.ParseFormat(format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return "", "", "", 2
	}
	return fs.Arg(0), fs.Arg(1), f, -1
}

// validateExtractKind rejects an unknown kind or one not assigned
// to the file.
func validateExtractKind(
	cfg *config.Config, res *config.FileResolution,
	kindName, path string,
) int {
	if _, declared := cfg.Kinds[kindName]; !declared {
		fmt.Fprintf(os.Stderr, "mdsmith: unknown kind %q\n", kindName)
		return 2
	}
	if !kindAssigned(res.Kinds, kindName) {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q is not assigned to %s\n", kindName, path)
		return 2
	}
	return 0
}

// gateExtractCheck runs the full check on the file and mirrors its
// exit semantics: a non-conformant file prints the same diagnostics
// and never reaches projection.
func gateExtractCheck(
	cfg *config.Config, cfgPath, path string, maxBytes int64,
) int {
	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		RootDir:          rootDirFromConfig(cfgPath),
		MaxInputBytes:    maxBytes,
		ConfigPath:       cfgPath,
	}
	result := runner.Run([]string{path})
	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		printErrors(result.Errors)
		return 2
	}
	if len(result.Diagnostics) > 0 {
		formatDiagnostics(result.Diagnostics, "text", false)
		return 1
	}
	return 0
}

// loadExtractFile reads and parses the document the same way the
// engine does so the match tree's line math lines up.
func loadExtractFile(
	cfg *config.Config, cfgPath, path string, maxBytes int64,
) (*lint.File, []byte, int) {
	source, err := lint.ReadFileLimited(path, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading %s: %v\n", path, err)
		return nil, nil, 2
	}
	f, err := lint.NewFileFromSource(path, source, frontMatterEnabled(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: parsing %s: %v\n", path, err)
		return nil, nil, 2
	}
	f.MaxInputBytes = maxBytes
	if rd := rootDirFromConfig(cfgPath); rd != "" {
		f.SetRootDir(rd)
	}
	return f, source, 0
}

// composedSchemaFor builds the same composed schema MDS020
// validates against, from the file's resolved required-structure
// config.
func composedSchemaFor(
	f *lint.File, res *config.FileResolution, kindName string,
) (*schema.Schema, int) {
	rsRule := &requiredstructure.Rule{}
	if rr, ok := res.Rules["required-structure"]; ok && rr.Final.Settings != nil {
		if err := rsRule.ApplySettings(rr.Final.Settings); err != nil {
			fmt.Fprintf(os.Stderr,
				"mdsmith: loading schema config: %v\n", err)
			return nil, 2
		}
	}
	sch, err := rsRule.ComposedSchema(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, 2
	}
	if sch == nil || sch.IsEmpty() {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q declares no schema to extract against\n",
			kindName)
		return nil, 2
	}
	return sch, 0
}

// kindAssigned reports whether name is one of the file's resolved
// kinds.
func kindAssigned(kinds []config.ResolvedKind, name string) bool {
	for _, k := range kinds {
		if k.Name == name {
			return true
		}
	}
	return false
}
