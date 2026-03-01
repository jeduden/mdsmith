package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/lint"
	metricspkg "github.com/jeduden/mdsmith/internal/metrics"
)

const metricsUsageText = `Usage: mdsmith metrics <command> [flags] [files...]

Commands:
  list     List available metrics from the shared registry
  rank     Rank files by selected metrics
`

func runMetrics(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, metricsUsageText)
		return 0
	}

	switch args[0] {
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
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json")
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
		return 2
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
	switch format {
	case "text":
		if err := writeMetricsListText(defs); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
			return 2
		}
	case "json":
		if err := writeMetricsListJSON(defs); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
			return 2
		}
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: unknown format %q (supported: text, json)\n", format)
		return 2
	}

	return 0
}

type metricsRankOptions struct {
	configPath       string
	metricsRaw       string
	byRaw            string
	orderRaw         string
	top              int
	format           string
	noGitignore      bool
	noFollowSymlinks bool
}

func runMetricsRank(args []string) int {
	opts, fileArgs, err := parseMetricsRankOptions(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return executeMetricsRank(opts, fileArgs)
}

func parseMetricsRankOptions(args []string) (metricsRankOptions, []string, error) {
	fs := flag.NewFlagSet("metrics rank", flag.ContinueOnError)
	var opts metricsRankOptions

	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVar(&opts.metricsRaw, "metrics", "", "Comma-separated metrics (defaults to registry defaults)")
	fs.StringVar(&opts.byRaw, "by", "", "Metric to sort by")
	fs.StringVar(&opts.orderRaw, "order", "", "Sort order: asc or desc (defaults by metric)")
	fs.IntVar(&opts.top, "top", 0, "Limit results to top N files (0 = all)")
	fs.StringVarP(&opts.format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&opts.noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&opts.noFollowSymlinks, "no-follow-symlinks", false, "Skip symbolic links when walking directories")

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
		return metricsRankOptions{}, nil, fmt.Errorf("--top must be >= 0")
	}

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

	files, err := resolveRankFiles(opts, fileArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	rows, err := metricspkg.Collect(files, defs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	metricspkg.SortRows(rows, byDef, order)
	rows = metricspkg.LimitRows(rows, opts.top)

	if err := writeRankOutput(opts.format, rows, defs); err != nil {
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

func resolveRankFiles(opts metricsRankOptions, fileArgs []string) ([]string, error) {
	cfg, _, err := loadConfig(opts.configPath)
	if err != nil {
		return nil, err
	}

	resolveOptions := resolveOpts(cfg, opts.noGitignore, opts.noFollowSymlinks)
	return lint.ResolveFilesWithOpts(fileArgs, resolveOptions)
}

func writeRankOutput(
	format string,
	rows []metricspkg.Row,
	defs []metricspkg.Definition,
) error {
	switch format {
	case "text":
		return writeMetricsRankText(rows, defs)
	case "json":
		return writeMetricsRankJSON(rows, defs)
	default:
		return fmt.Errorf("unknown format %q (supported: text, json)", format)
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

func writeMetricsListText(defs []metricspkg.Definition) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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

func writeMetricsListJSON(defs []metricspkg.Definition) error {
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func writeMetricsRankText(rows []metricspkg.Row, defs []metricspkg.Definition) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	var headers []string
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

func writeMetricsRankJSON(rows []metricspkg.Row, defs []metricspkg.Definition) error {
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
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
