package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/index"
)

// depRecord is one dependency edge, either an outgoing reference from
// the queried file or an incoming reference to it.
type depRecord struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// depsOptions bundles the parsed CLI flags for `deps`.
type depsOptions struct {
	configPath   string
	format       string
	maxInputSize string
	incoming     bool
	walk         walkCLI
}

// edgeKindString maps an index.EdgeKind to the stable label the CLI
// and JSON output expose.
func edgeKindString(k index.EdgeKind) string {
	switch k {
	case index.EdgeAnchorLink:
		return "anchor-link"
	case index.EdgeFileLink:
		return "file-link"
	case index.EdgeRefLink:
		return "ref-link"
	case index.EdgeInclude:
		return "include"
	case index.EdgeCatalog:
		return "catalog"
	case index.EdgeBuild:
		return "build"
	default:
		return "unknown"
	}
}

// edgeTargetString renders an edge's target for display. selfFile is
// the file the edge originates in, used to render same-file anchor
// references as `#anchor` rather than a bare empty path.
func edgeTargetString(e index.Edge, selfFile string) string {
	if e.Unresolved {
		return "(glob)"
	}
	if e.TargetLabel != "" && e.TargetFile == "" {
		return "[" + e.TargetLabel + "]"
	}
	if e.TargetFile == "" {
		if e.TargetAnchor != "" {
			return "#" + e.TargetAnchor
		}
		return selfFile
	}
	if e.TargetAnchor != "" {
		return e.TargetFile + "#" + e.TargetAnchor
	}
	return e.TargetFile
}

// collectDeps returns the dependency records for target. When incoming
// is false it lists edges originating in target (what target depends
// on); when true it lists edges pointing at target (what depends on
// target).
func collectDeps(idx *index.Index, target string, incoming bool) []depRecord {
	var recs []depRecord
	if incoming {
		for _, e := range idx.BacklinksFor(target) {
			recs = append(recs, depRecord{
				Source: e.SourceFile,
				Line:   e.SourceLine,
				Kind:   edgeKindString(e.Kind),
				Target: target,
			})
		}
		return recs
	}
	for _, e := range idx.OutgoingEdges(target) {
		recs = append(recs, depRecord{
			Source: target,
			Line:   e.SourceLine,
			Kind:   edgeKindString(e.Kind),
			Target: edgeTargetString(e, target),
		})
	}
	sort.SliceStable(recs, func(a, b int) bool {
		if recs[a].Line != recs[b].Line {
			return recs[a].Line < recs[b].Line
		}
		return recs[a].Target < recs[b].Target
	})
	return recs
}

// emitDeps writes records to w. Exit code: 0 when records were
// emitted, 1 when none, 2 on unknown format or write error.
func emitDeps(w io.Writer, recs []depRecord, format string) int {
	switch format {
	case "json":
		out := recs
		if out == nil {
			out = []depRecord{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: writing json: %v\n", err)
			return 2
		}
	case "text", "":
		for _, r := range recs {
			if _, err := fmt.Fprintf(w, "%s:%d: %s %s\n", r.Source, r.Line, r.Kind, r.Target); err != nil {
				fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
				return 2
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: unknown --format %q (want text or json)\n", format)
		return 2
	}
	if len(recs) == 0 {
		return 1
	}
	return 0
}

// parseDepsFlags parses the flags for `mdsmith deps` and returns the
// options plus the remaining positional arguments.
func parseDepsFlags(args []string) (depsOptions, []string, error) {
	fs := flag.NewFlagSet("deps", flag.ContinueOnError)
	var (
		opts                        depsOptions
		noGitignore, followSymlinks bool
	)
	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&opts.format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&opts.incoming, "incoming", false,
		"List files that depend on <file> instead of what <file> depends on")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith deps [flags] <file>\n\n"+
			"List the dependency edges of <file>: the includes, catalogs,\n"+
			"build inputs, and links it points at. With --incoming, list\n"+
			"every workspace file that points at <file> instead.\n\n"+
			"Exit codes: 0 found, 1 none, 2 error\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	opts.walk = walkCLI{
		noGitignore:    noGitignore,
		followSymlinks: followSymlinksOverride(fs, followSymlinks),
	}
	return opts, fs.Args(), nil
}

// runDeps implements the "deps" subcommand: show the dependency graph
// edges for one file, in either direction.
func runDeps(args []string) int {
	opts, posArgs, err := parseDepsFlags(args)
	if err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: deps"); code >= 0 {
			return code
		}
	}
	if len(posArgs) != 1 {
		fmt.Fprint(os.Stderr, "mdsmith: deps requires exactly one <file> argument\n")
		return 2
	}
	target := normalizeWorkspacePath(posArgs[0])
	if !isWorkspaceRelativeTarget(target) {
		fmt.Fprintf(os.Stderr, "mdsmith: target %q must be workspace-relative\n", target)
		return 2
	}

	cfg, cfgPath, _, files, code := discoverFiles(opts.configPath, false, opts.walk)
	if code >= 0 {
		if code == 0 {
			return emitDeps(os.Stdout, nil, opts.format)
		}
		return code
	}
	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	rootDir := rootDirFromConfig(cfgPath)
	relToAbs := make(map[string]string, len(files))
	rels := make([]string, 0, len(files))
	for _, src := range files {
		rel := index.NormalizePath(workspaceRelativePath(src, rootDir))
		relToAbs[rel] = src
		rels = append(rels, rel)
	}
	idx := index.New(rootDir)
	idx.BuildSerial(rels, func(rel string) ([]byte, error) {
		// rel always comes from rels, and rels is built in
		// lockstep with relToAbs, so the lookup never misses.
		return bytelimit.ReadFileLimited(relToAbs[rel], maxBytes)
	})

	recs := collectDeps(idx, target, opts.incoming)
	return emitDeps(os.Stdout, recs, opts.format)
}
