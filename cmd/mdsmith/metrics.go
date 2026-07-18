package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	metricspkg "github.com/jeduden/mdsmith/internal/metrics"
)

const metricsUsageText = `Usage: mdsmith metrics <command> [flags] [files...]

Commands:
  get      Emit all metrics for a single file as a data object
  list     List available metrics from the shared registry
  rank     Rank files by selected metrics
`

func runMetrics(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, metricsUsageText)
		return 0
	}

	switch args[0] {
	case "get":
		return runMetricsGet(args[1:])
	case "list":
		return runMetricsList(args[1:])
	case "rank":
		return runMetricsRank(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: metrics: unknown command %q\n", args[0])
		return 2
	}
}

func runMetricsList(args []string) int {
	fs := flag.NewFlagSet("metrics list", flag.ContinueOnError)
	var (
		scopeRaw string
		format   string
	)

	fs.StringVar(&scopeRaw, "scope", "file", "Metric scope: file")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json, yaml")
	fs.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			"Usage: mdsmith metrics list [flags]\n\n"+
				"List available metrics in the shared registry.\n\n"+
				"Flags:\n",
		)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: metrics list"); code >= 0 {
			return code
		}
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: metrics list takes no file arguments\n")
		return 2
	}

	scope, err := metricspkg.ParseScope(scopeRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	defs := metricspkg.ForScope(scope)
	if err := writeListOutput(os.Stdout, format, defs); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

func writeListOutput(w io.Writer, format string, defs []metricspkg.Definition) error {
	switch format {
	case "text":
		return writeMetricsListText(w, defs)
	case "json":
		return writeMetricsListJSON(w, defs)
	case "yaml":
		return writeMetricsListYAML(w, defs)
	default:
		return fmt.Errorf("unknown format %q (supported: text, json, yaml)", format)
	}
}

type metricsRankOptions struct {
	configPath     string
	metricsRaw     string
	byRaw          string
	orderRaw       string
	top            int
	format         string
	noGitignore    bool
	followSymlinks *bool
	maxInputSize   string
}

func runMetricsRank(args []string) int {
	opts, fileArgs, err := parseMetricsRankOptions(args)
	if err != nil {
		// parseMetricsRankOptions returns both flag.ErrHelp from
		// fs.Parse and post-parse validation errors (e.g. --top
		// < 0). reportFlagParseErr handles the help/parse cases
		// with the right prefix; everything else gets surfaced
		// with the same shape here.
		return reportFlagParseErr(err, os.Stderr, "mdsmith: metrics rank")
	}
	return executeMetricsRank(opts, fileArgs)
}

func parseMetricsRankOptions(args []string) (metricsRankOptions, []string, error) {
	fs := flag.NewFlagSet("metrics rank", flag.ContinueOnError)
	var opts metricsRankOptions
	var followSymlinks bool

	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVar(&opts.metricsRaw, "metrics", "", "Comma-separated metrics (defaults to registry defaults)")
	fs.StringVar(&opts.byRaw, "by", "", "Metric to sort by")
	fs.StringVar(&opts.orderRaw, "order", "", "Sort order: asc or desc (defaults by metric)")
	fs.IntVar(&opts.top, "top", 0, "Limit results to top N files (0 = all)")
	fs.StringVarP(&opts.format, "format", "f", "text", "Output format: text, json, yaml")
	fs.BoolVar(&opts.noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			"Usage: mdsmith metrics rank [flags] [files...]\n\n"+
				"Compute selected metrics and rank Markdown files.\n"+
				"With no file arguments, defaults to the current directory.\n\n"+
				"Flags:\n",
		)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return metricsRankOptions{}, nil, err
	}
	if opts.top < 0 {
		return metricsRankOptions{}, nil, errors.New("--top must be >= 0")
	}
	opts.followSymlinks = followSymlinksOverride(fs, followSymlinks)

	fileArgs := fs.Args()
	if len(fileArgs) == 0 {
		fileArgs = []string{"."}
	}

	return opts, fileArgs, nil
}

func executeMetricsRank(opts metricsRankOptions, fileArgs []string) int {
	defs, byDef, order, err := resolveRankSelection(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	cfg, _, err := loadConfig(opts.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	files, err := resolveRankFiles(cfg, opts, fileArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	rows, err := metricspkg.Collect(files, defs, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	metricspkg.SortRows(rows, byDef, order)
	rows = metricspkg.LimitRows(rows, opts.top)

	if err := writeRankOutput(os.Stdout, opts.format, rows, defs); err != nil {
		if strings.Contains(err.Error(), "unknown format") {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return 2
		}
		fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
		return 2
	}

	return 0
}

func resolveRankSelection(
	opts metricsRankOptions,
) ([]metricspkg.Definition, metricspkg.Definition, metricspkg.Order, error) {
	scope := metricspkg.ScopeFile
	selectedNames := metricspkg.SplitList(opts.metricsRaw)
	defs, err := metricspkg.Resolve(scope, selectedNames)
	if err != nil {
		return nil, metricspkg.Definition{}, "", err
	}

	var byDef metricspkg.Definition
	if strings.TrimSpace(opts.byRaw) == "" {
		byDef = defs[0]
	} else {
		byDefs, err := metricspkg.Resolve(scope, []string{opts.byRaw})
		if err != nil {
			return nil, metricspkg.Definition{}, "", err
		}
		byDef = byDefs[0]
	}

	// Ensure the sort metric is always computed.
	if !containsMetric(defs, byDef.ID) {
		if len(selectedNames) > 0 {
			return nil, metricspkg.Definition{}, "", fmt.Errorf(
				"--by metric %q must be included in --metrics",
				byDef.Name,
			)
		}
		defs = append(defs, byDef)
	}

	order := byDef.DefaultOrder
	if strings.TrimSpace(opts.orderRaw) != "" {
		parsed, err := metricspkg.ParseOrder(opts.orderRaw)
		if err != nil {
			return nil, metricspkg.Definition{}, "", err
		}
		order = parsed
	}

	return defs, byDef, order, nil
}

func resolveRankFiles(cfg *config.Config, opts metricsRankOptions, fileArgs []string) ([]string, error) {
	resolveOptions := resolveOpts(cfg, walkCLI{
		noGitignore:    opts.noGitignore,
		followSymlinks: opts.followSymlinks,
	})
	return lint.ResolveFilesWithOpts(fileArgs, resolveOptions)
}

func writeRankOutput(
	w io.Writer,
	format string,
	rows []metricspkg.Row,
	defs []metricspkg.Definition,
) error {
	switch format {
	case "text":
		return writeMetricsRankText(w, rows, defs)
	case "json":
		return writeMetricsRankJSON(w, rows, defs)
	case "yaml":
		return writeMetricsRankYAML(w, rows, defs)
	default:
		return fmt.Errorf("unknown format %q (supported: text, json, yaml)", format)
	}
}

func containsMetric(defs []metricspkg.Definition, id string) bool {
	for _, def := range defs {
		if def.ID == id {
			return true
		}
	}
	return false
}

func writeMetricsListText(w io.Writer, defs []metricspkg.Definition) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tNAME\tSCOPE\tORDER\tDEFAULT\tDESCRIPTION"); err != nil {
		return err
	}
	for _, def := range defs {
		if _, err := fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%t\t%s\n",
			def.ID,
			def.Name,
			def.Scope,
			def.DefaultOrder,
			def.Default,
			def.Description,
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeMetricsListJSON(w io.Writer, defs []metricspkg.Definition) error {
	items := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		items = append(items, map[string]any{
			"id":            def.ID,
			"name":          def.Name,
			"description":   def.Description,
			"scope":         def.Scope,
			"default":       def.Default,
			"default_order": def.DefaultOrder,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func writeMetricsRankText(w io.Writer, rows []metricspkg.Row, defs []metricspkg.Definition) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	headers := make([]string, 0, len(defs)+1)
	for _, def := range defs {
		headers = append(headers, strings.ToUpper(def.Name))
	}
	headers = append(headers, "PATH")
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}

	for _, row := range rows {
		cols := make([]string, 0, len(defs)+1)
		for _, def := range defs {
			cols = append(cols, metricspkg.FormatValue(def, row.Metrics[def.Name]))
		}
		cols = append(cols, row.Path)
		if _, err := fmt.Fprintln(tw, strings.Join(cols, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func writeMetricsRankJSON(w io.Writer, rows []metricspkg.Row, defs []metricspkg.Definition) error {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"path": row.Path,
		}
		for _, def := range defs {
			item[def.Name] = metricspkg.JSONValue(def, row.Metrics[def.Name])
		}
		items = append(items, item)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func writeMetricsListYAML(w io.Writer, defs []metricspkg.Definition) error {
	items := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		items = append(items, map[string]any{
			"id":            def.ID,
			"name":          def.Name,
			"description":   def.Description,
			"scope":         def.Scope,
			"default":       def.Default,
			"default_order": def.DefaultOrder,
		})
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(items); err != nil {
		return err
	}
	return enc.Close()
}

func writeMetricsRankYAML(w io.Writer, rows []metricspkg.Row, defs []metricspkg.Definition) error {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"path": row.Path,
		}
		for _, def := range defs {
			item[def.Name] = metricspkg.JSONValue(def, row.Metrics[def.Name])
		}
		items = append(items, item)
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(items); err != nil {
		return err
	}
	return enc.Close()
}

func runMetricsGet(args []string) int {
	fs := flag.NewFlagSet("metrics get", flag.ContinueOnError)
	var format string

	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json, yaml")
	fs.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			"Usage: mdsmith metrics get [flags] <file>\n\n"+
				"Emit all registered metrics for a single Markdown file.\n\n"+
				"Flags:\n",
		)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: metrics get"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "mdsmith: metrics get requires exactly one file argument\n")
		return 2
	}
	path := fs.Arg(0)

	defs := metricspkg.ForScope(metricspkg.ScopeFile)
	item, err := collectGetItem(path, defs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	if err := writeGetOutput(os.Stdout, format, item, defs); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
		return 2
	}
	return 0
}

func collectGetItem(path string, defs []metricspkg.Definition) (map[string]any, error) {
	source, err := bytelimit.ReadFileLimited(path, bytelimit.DefaultMaxInputBytes)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", path, err)
	}

	doc := metricspkg.NewDocument(path, gensection.AuthoredSource(source))
	item := map[string]any{"path": path}
	for _, def := range defs {
		v, err := def.Compute(doc)
		if err != nil {
			return nil, fmt.Errorf("computing %q for %q: %w", def.Name, path, err)
		}
		item[def.Name] = metricspkg.JSONValue(def, v)
	}
	return item, nil
}

func writeGetOutput(w io.Writer, format string, item map[string]any, defs []metricspkg.Definition) error {
	switch format {
	case "json":
		return writeMetricsGetJSON(w, item)
	case "yaml":
		return writeMetricsGetYAML(w, item)
	case "text":
		return writeMetricsGetText(w, item, defs)
	default:
		return fmt.Errorf("unknown format %q (supported: text, json, yaml)", format)
	}
}

func writeMetricsGetJSON(w io.Writer, item map[string]any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(item)
}

func writeMetricsGetYAML(w io.Writer, item map[string]any) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(item); err != nil {
		return err
	}
	return enc.Close()
}

func writeMetricsGetText(w io.Writer, item map[string]any, defs []metricspkg.Definition) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tVALUE"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "path\t%s\n", item["path"]); err != nil {
		return err
	}
	for _, def := range defs {
		v := item[def.Name]
		var display string
		if v == nil {
			display = "-"
		} else {
			switch n := v.(type) {
			case float64:
				display = fmt.Sprintf("%.*f", def.Precision, n)
			case int64:
				display = strconv.FormatInt(n, 10)
			default:
				display = "-"
			}
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", def.Name, display); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func runHelpMetrics(args []string) int {
	if len(args) == 0 {
		return listAllMetrics()
	}
	return showMetric(args[0])
}

func listAllMetrics() int {
	metrics, err := metricspkg.ListDocs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	for _, m := range metrics {
		fmt.Printf("%-6s %-20s %s\n", m.ID, m.Name, m.Description)
	}
	return 0
}

func showMetric(query string) int {
	content, err := metricspkg.LookupDoc(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	fmt.Print(content)
	return 0
}
