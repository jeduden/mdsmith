package main

import (
	"bytes"
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/query"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// runQuery implements the "query" subcommand: select files by CUE
// expression on front matter.
type queryOptions struct {
	nul          bool
	verbose      bool
	configPath   string
	maxInputSize string
}

func parseQueryFlags(args []string) (queryOptions, []string, error) {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	var opts queryOptions

	fs.BoolVarP(&opts.nul, "null", "0", false, "NUL-delimit output (for xargs -0)")
	fs.BoolVarP(&opts.verbose, "verbose", "v", false, "Print skipped files and reasons on stderr")
	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith list query [flags] <cue-expr> [files...]\n\n"+
			"Print paths of Markdown files whose front matter satisfies a CUE expression.\n"+
			"With no file arguments, searches the current directory recursively.\n\n"+
			"Exit codes: 0 match, 1 no match, 2 error\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	return opts, fs.Args(), nil
}

func runQuery(args []string) int {
	opts, posArgs, err := parseQueryFlags(args)
	if err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: list query"); code >= 0 {
			return code
		}
	}

	if len(posArgs) == 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: list query requires a CUE expression argument\n")
		return 2
	}

	expr := posArgs[0]
	fileArgs := posArgs[1:]

	matcher, err := query.Compile(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	if len(fileArgs) == 0 {
		fileArgs = []string{"."}
	}

	files, err := lint.ResolveFilesWithOpts(fileArgs, lint.ResolveOpts{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	cfg, _, err := loadConfig(opts.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	delim := "\n"
	if opts.nul {
		delim = "\x00"
	}

	matched := queryFiles(matcher, files, delim, opts.verbose, maxBytes)
	if matched > 0 {
		return 0
	}
	return 1
}

// queryFiles tests each file against matcher and writes matching paths
// to stdout. Returns the number of matches.
func queryFiles(matcher *query.Matcher, files []string, delim string, verbose bool, maxBytes int64) int {
	matched := 0
	for _, f := range files {
		fm, err := readFrontMatterRaw(f, maxBytes)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "skip %s: %v\n", f, err)
			}
			continue
		}
		if fm == nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "skip %s: no front matter\n", f)
			}
			continue
		}
		if matcher.Match(fm) {
			_, _ = fmt.Fprintf(os.Stdout, "%s%s", f, delim)
			matched++
		} else if verbose {
			fmt.Fprintf(os.Stderr, "skip %s: expression not satisfied\n", f)
		}
	}
	return matched
}

// readFrontMatterRaw reads a file, strips front matter, and
// unmarshals YAML into map[string]any (preserving numeric types).
func readFrontMatterRaw(path string, maxBytes int64) (map[string]any, error) {
	data, err := bytelimit.ReadFileLimited(path, maxBytes)
	if err != nil {
		return nil, err
	}
	prefix, _ := lint.StripFrontMatter(data)
	if prefix == nil {
		return nil, nil
	}
	// Strip the --- delimiters to get the YAML body.
	delim := []byte("---\n")
	yamlBytes := bytes.TrimSuffix(bytes.TrimPrefix(prefix, delim), delim)

	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
		return nil, fmt.Errorf("parsing front matter: %w", err)
	}
	// Distinguish empty front matter (---\n---\n) from absent front matter.
	// An empty YAML document unmarshals to nil; normalize to an empty map
	// so the caller only sees nil when no front matter block exists.
	if raw == nil {
		raw = make(map[string]any)
	}
	return raw, nil
}
