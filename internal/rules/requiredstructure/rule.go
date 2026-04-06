package requiredstructure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that a document's heading structure matches a schema.
type Rule struct {
	Schema string // path to schema file
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS020" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "required-structure" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "schema":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("required-structure: schema must be a string, got %T", v)
			}
			r.Schema = s
		default:
			return fmt.Errorf("required-structure: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"schema": "",
	}
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	// Warn when <?require?> appears in a non-schema file.
	if reqLine := findRequireDirectiveLine(f); reqLine > 0 {
		if r.Schema == "" || !isSchemaFile(f.Path, r.Schema) {
			d := makeDiag(f.Path, reqLine,
				"<?require?> is only recognized in schema files; this directive has no effect here")
			d.Severity = lint.Warning
			diags = append(diags, d)
		}
	}

	if r.Schema == "" {
		return diags
	}

	schData, err := os.ReadFile(r.Schema)
	if err != nil {
		return append(diags, r.diag(f.Path, 1,
			fmt.Sprintf("cannot read schema %q: %v", r.Schema, err)))
	}

	sch, err := parseSchema(schData, r.Schema)
	if err != nil {
		return append(diags, r.diag(f.Path, 1,
			fmt.Sprintf("invalid schema %q: %v", r.Schema, err)))
	}

	// Skip the schema file itself.
	if isSchemaFile(f.Path, r.Schema) {
		return diags
	}

	docHeadings := extractHeadings(f)
	docFMRaw, fmDiags := readDocFrontMatterRaw(f)
	diags = append(diags, fmDiags...)

	// Check filename pattern.
	diags = append(diags, checkFilenamePattern(f, sch)...)

	// Check structure: required headings present and in order.
	diags = append(diags, checkStructure(f, sch, docHeadings)...)

	// Validate document front matter against schema-embedded CUE constraints.
	if err := validateFrontMatterCUE(sch.Config.FrontMatterCUE, docFMRaw); err != nil {
		diags = append(diags, makeDiag(f.Path, 1,
			fmt.Sprintf("front matter does not satisfy schema CUE constraints: %v", err)))
	}

	// Check frontmatter-body sync using raw map for nested access.
	diags = append(diags, checkSync(f, sch, docHeadings, docFMRaw)...)

	return diags
}

func (r *Rule) diag(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     file,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Error,
		Message:  msg,
	}
}

var _ rule.Configurable = (*Rule)(nil)

// schemaConfig holds the parsed schema frontmatter.
type schemaConfig struct {
	FrontMatterCUE  string
	FilenamePattern string // glob pattern the document basename must match
}

// schemaHeading represents a required heading from the schema.
type schemaHeading struct {
	Level int
	Text  string // raw text, may contain {field} or ?
}

// parsedSchema holds the full parsed schema.
type parsedSchema struct {
	Config   schemaConfig
	Headings []schemaHeading
	// syncPoints maps heading index to list of (field, expected text) pairs
	// for body sync checking.
	SyncPoints map[int][]syncPoint
}

// syncPoint represents a {field} reference in heading text.
type syncPoint struct {
	Field    string
	InBody   bool   // true if in body content, false if in heading
	BodyText string // the full expected body line text with field substituted
}

var cueIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const sectionWildcard = "..."

// parseSchemaFrontMatter extracts the schema configuration from frontmatter.
func parseSchemaFrontMatter(prefix []byte) (schemaConfig, error) {
	cfg := schemaConfig{}
	if prefix == nil {
		return cfg, nil
	}
	yamlBytes := extractYAML(prefix)
	derivedSchema, err := deriveFrontMatterCUE(yamlBytes)
	if err != nil {
		return cfg, err
	}
	cfg.FrontMatterCUE = derivedSchema
	if err := validateCUESchemaSyntax(cfg.FrontMatterCUE); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// extractRequireDirective walks the schema AST for a <?require?> PI
// and parses its YAML body to extract constraints like filename.
func extractRequireDirective(f *lint.File) (string, error) {
	var filenamePattern string
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		pi, ok := c.(*lint.ProcessingInstruction)
		if !ok || pi.Name != "require" {
			continue
		}
		// Extract YAML body from PI content.
		lines := pi.Lines()
		var body []byte
		if lines.Len() == 1 {
			// Single-line: <?require key: value ?>
			// Extract content between <?require and ?>
			// (ignoring any trailing text after ?>).
			seg := lines.At(0)
			line := strings.TrimSpace(string(seg.Value(f.Source)))
			line = strings.TrimPrefix(line, "<?require")
			if idx := strings.Index(line, "?>"); idx >= 0 {
				line = line[:idx]
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			body = []byte(line)
		} else {
			// Multi-line: skip first line (<?require),
			// remaining lines are YAML body before ?>
			for i := 1; i < lines.Len(); i++ {
				seg := lines.At(i)
				body = append(body, seg.Value(f.Source)...)
			}
		}
		if err := lint.RejectYAMLAliases(body); err != nil {
			return "", fmt.Errorf("invalid <?require?> directive: %w", err)
		}
		var params map[string]string
		if err := yaml.Unmarshal(body, &params); err != nil {
			return "", fmt.Errorf("invalid <?require?> directive: %w", err)
		}
		if fn, ok := params["filename"]; ok {
			filenamePattern = fn
		}
		break
	}
	return filenamePattern, nil
}

func deriveFrontMatterCUE(yamlBytes []byte) (string, error) {
	if err := lint.RejectYAMLAliases(yamlBytes); err != nil {
		return "", fmt.Errorf("parsing schema frontmatter: %w", err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return "", fmt.Errorf("parsing schema frontmatter: %w", err)
	}
	if len(raw) == 0 {
		return "", nil
	}

	expr, err := cueExprForMap(raw)
	if err != nil {
		return "", fmt.Errorf("parsing schema frontmatter constraints: %w", err)
	}
	return "close(" + expr + ")", nil
}

func cueExprForMap(m map[string]any) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("{\n")
	for _, k := range keys {
		expr, err := cueExprForValue(m[k])
		if err != nil {
			return "", fmt.Errorf("field %q: %w", k, err)
		}
		b.WriteString("  ")
		fieldName, optional := strings.CutSuffix(k, "?")
		b.WriteString(cueFieldLabel(fieldName))
		if optional {
			b.WriteString("?")
		}
		b.WriteString(": ")
		b.WriteString(expr)
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String(), nil
}

func cueExprForValue(v any) (string, error) {
	switch x := v.(type) {
	case map[string]any:
		return cueExprForMap(x)
	case []any:
		b, err := json.Marshal(x)
		if err != nil {
			return "", fmt.Errorf("marshal array value: %w", err)
		}
		return string(b), nil
	case string:
		expr := strings.TrimSpace(x)
		if expr == "" {
			return "", fmt.Errorf("schema expression must be non-empty")
		}
		return expr, nil
	case int, int64, float64, bool:
		b, err := json.Marshal(x)
		if err != nil {
			return "", fmt.Errorf("marshal scalar value: %w", err)
		}
		return string(b), nil
	default:
		return "", fmt.Errorf("unsupported schema value type %T", v)
	}
}

func cueFieldLabel(key string) string {
	if cueIdentPattern.MatchString(key) {
		return key
	}
	return strconv.Quote(key)
}

// collectBodySyncPoints scans body content for {field} references and
// adds them to the syncPoints map under their nearest preceding heading.
func collectBodySyncPoints(
	content []byte, headings []docHeading,
	syncPoints map[int][]syncPoint,
) {
	lines := strings.Split(string(content), "\n")
	currentHeading := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			for j, h := range headings {
				if headingMatchesLine(h, trimmed) {
					currentHeading = j
					break
				}
			}
			continue
		}
		if currentHeading >= 0 && trimmed != "" {
			fields := fieldinterp.Fields(trimmed)
			for _, f := range fields {
				syncPoints[currentHeading] = append(
					syncPoints[currentHeading],
					syncPoint{Field: f, InBody: true, BodyText: trimmed},
				)
			}
		}
	}
}

// maxSchemaIncludeDepth is the maximum nesting depth for schema includes.
const maxSchemaIncludeDepth = 10

// parseSchema reads schema bytes, extracts frontmatter config and
// required headings. When schemaPath is non-empty, <?include?> directives
// are expanded and their headings spliced in.
func parseSchema(data []byte, schemaPath string) (*parsedSchema, error) {
	prefix, content := lint.StripFrontMatter(data)

	cfg, err := parseSchemaFrontMatter(prefix)
	if err != nil {
		return nil, err
	}

	f, err := lint.NewFile("schema", content)
	if err != nil {
		return nil, fmt.Errorf("parsing schema markdown: %w", err)
	}

	// Extract <?require?> directive from schema body.
	filenamePattern, err := extractRequireDirective(f)
	if err != nil {
		return nil, err
	}
	cfg.FilenamePattern = filenamePattern

	// Extract headings, expanding <?include?> directives in the schema.
	var headings []docHeading
	if schemaPath != "" {
		cleanPath := filepath.Clean(schemaPath)
		visited := map[string]bool{cleanPath: true}
		chain := []string{cleanPath}
		var fp string
		headings, fp, err = extractSchemaHeadings(f, schemaPath, visited, chain)
		if err != nil {
			return nil, err
		}
		if fp != "" && cfg.FilenamePattern == "" {
			cfg.FilenamePattern = fp
		}
	} else {
		headings = extractHeadings(f)
	}

	schHeadings := make([]schemaHeading, len(headings))
	syncPoints := make(map[int][]syncPoint)

	for i, h := range headings {
		schHeadings[i] = schemaHeading{Level: h.Level, Text: h.Text}
		for _, f := range fieldinterp.Fields(h.Text) {
			syncPoints[i] = append(syncPoints[i], syncPoint{Field: f})
		}
	}

	collectBodySyncPoints(content, headings, syncPoints)

	return &parsedSchema{
		Config:     cfg,
		Headings:   schHeadings,
		SyncPoints: syncPoints,
	}, nil
}

// extractSchemaHeadings walks the schema AST, collecting headings and
// expanding <?include?> PIs by splicing in the included file's headings.
// It uses a visited set for cycle detection.
func extractSchemaHeadings(
	schemaFile *lint.File, schemaPath string,
	visited map[string]bool, chain []string,
) ([]docHeading, string, error) {
	var headings []docHeading
	var filenamePattern string

	err := ast.Walk(schemaFile.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			text := headingText(node, schemaFile.Source)
			line := schemaFile.LineOfOffset(node.Lines().At(0).Start)
			headings = append(headings, docHeading{Level: node.Level, Text: text, Line: line})

		case *lint.ProcessingInstruction:
			if node.Name != "include" {
				return ast.WalkContinue, nil
			}
			fragHeadings, fp, walkErr := expandSchemaInclude(
				node, schemaFile.Source, schemaPath, visited, chain)
			if walkErr != nil {
				return ast.WalkStop, walkErr
			}
			if fp != "" && filenamePattern == "" {
				filenamePattern = fp
			}
			headings = append(headings, fragHeadings...)
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, "", err
	}

	return headings, filenamePattern, nil
}

// expandSchemaInclude resolves a single <?include?> PI in a schema file,
// reads the fragment, and returns its headings and any filename pattern.
// resolveSchemaIncludePath extracts and validates the file parameter from
// an include PI, returning the resolved filesystem path.
func resolveSchemaIncludePath(
	pi *lint.ProcessingInstruction, source []byte, schemaPath string,
) (string, error) {
	fileParam, err := extractPIFileParam(pi, source)
	if err != nil {
		return "", fmt.Errorf("parsing include processing instruction: %w", err)
	}
	if strings.TrimSpace(fileParam) == "" {
		return "", fmt.Errorf("include processing instruction missing required 'file' attribute")
	}
	if filepath.IsAbs(fileParam) {
		return "", fmt.Errorf("schema include has absolute file path %q", fileParam)
	}
	for _, elem := range strings.Split(filepath.ToSlash(fileParam), "/") {
		if elem == ".." {
			return "", fmt.Errorf(
				"schema include file path %q contains \"..\" traversal", fileParam)
		}
	}
	dir := filepath.Dir(schemaPath)
	return filepath.Clean(filepath.Join(dir, fileParam)), nil
}

func expandSchemaInclude(
	pi *lint.ProcessingInstruction, source []byte,
	schemaPath string, visited map[string]bool, chain []string,
) ([]docHeading, string, error) {
	includedPath, err := resolveSchemaIncludePath(pi, source, schemaPath)
	if err != nil {
		return nil, "", err
	}

	if len(chain) > maxSchemaIncludeDepth {
		return nil, "", fmt.Errorf(
			"schema include depth exceeds maximum (%d)", maxSchemaIncludeDepth)
	}
	if visited[includedPath] {
		chainCopy := make([]string, len(chain))
		copy(chainCopy, chain)
		chainCopy = append(chainCopy, includedPath)
		return nil, "", fmt.Errorf(
			"cyclic include: %s", strings.Join(chainCopy, " -> "))
	}

	fragData, err := os.ReadFile(includedPath)
	if err != nil {
		return nil, "", fmt.Errorf(
			"schema include file %q not found: %w", includedPath, err)
	}

	_, fragContent := lint.StripFrontMatter(fragData)
	fragFile, err := lint.NewFile(includedPath, fragContent)
	if err != nil {
		return nil, "", fmt.Errorf(
			"parsing schema include %q: %w", includedPath, err)
	}

	fp, err := extractRequireDirective(fragFile)
	if err != nil {
		return nil, "", err
	}

	visited[includedPath] = true
	chain = append(chain, includedPath)
	fragHeadings, fp2, err := extractSchemaHeadings(
		fragFile, includedPath, visited, chain)
	delete(visited, includedPath)
	if err != nil {
		return nil, "", err
	}
	if fp2 != "" && fp == "" {
		fp = fp2
	}

	return fragHeadings, fp, nil
}

// extractPIFileParam parses the YAML body of an include PI to extract
// the "file" parameter.
func extractPIFileParam(pi *lint.ProcessingInstruction, source []byte) (string, error) {
	lines := pi.Lines()
	var body string
	if lines.Len() == 1 {
		seg := lines.At(0)
		raw := strings.TrimSpace(string(seg.Value(source)))
		raw = strings.TrimPrefix(raw, "<?"+pi.Name)
		if idx := strings.Index(raw, "?>"); idx >= 0 {
			raw = raw[:idx]
		}
		body = strings.TrimSpace(raw)
	} else {
		var b strings.Builder
		for i := 1; i < lines.Len(); i++ {
			seg := lines.At(i)
			b.Write(seg.Value(source))
		}
		body = b.String()
	}

	if body == "" {
		return "", nil
	}

	if err := lint.RejectYAMLAliases([]byte(body)); err != nil {
		return "", fmt.Errorf("invalid include directive YAML: %w", err)
	}
	var params map[string]string
	if err := yaml.Unmarshal([]byte(body), &params); err != nil {
		return "", fmt.Errorf("invalid include directive YAML: %w", err)
	}

	return params["file"], nil
}

// docHeading represents a heading found in the document being checked.
type docHeading struct {
	Level int
	Text  string
	Line  int
}

// extractHeadings walks the AST and collects all headings.
func extractHeadings(f *lint.File) []docHeading {
	var headings []docHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		text := headingText(h, f.Source)
		line := f.LineOfOffset(h.Lines().At(0).Start)

		headings = append(headings, docHeading{
			Level: h.Level,
			Text:  text,
			Line:  line,
		})
		return ast.WalkContinue, nil
	})
	return headings
}

// headingText extracts the plain text content of a heading node.
func headingText(h *ast.Heading, source []byte) string {
	var buf strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, &buf)
	}
	return buf.String()
}

// writeNodeText recursively writes the text content of an AST node.
func writeNodeText(n ast.Node, source []byte, buf *strings.Builder) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	if _, ok := n.(*ast.CodeSpan); ok {
		// Code spans store text in child nodes.
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			writeNodeText(c, source, buf)
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, buf)
	}
}

// headingMatchesLine checks if a docHeading matches a raw heading line.
func headingMatchesLine(h docHeading, line string) bool {
	// Strip leading # and spaces.
	stripped := strings.TrimLeft(line, "#")
	stripped = strings.TrimSpace(stripped)
	return stripped == h.Text
}

// checkStructure verifies required headings are present and in order.
func checkStructure(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
) []lint.Diagnostic {
	var diags []lint.Diagnostic

	docIdx := 0
	allowExtra := false
	for _, req := range sch.Headings {
		if isSectionWildcard(req) {
			allowExtra = true
			continue
		}

		found := false
		for docIdx < len(docHeadings) {
			dh := docHeadings[docIdx]
			if matchesSchema(req, dh) {
				// Check level.
				if dh.Level != req.Level {
					diags = append(diags, makeDiag(f.Path, dh.Line,
						fmt.Sprintf("heading level mismatch: expected h%d, got h%d",
							req.Level, dh.Level)))
				}
				docIdx++
				found = true
				allowExtra = false
				break
			}
			if !allowExtra {
				diags = append(diags, makeDiag(f.Path, dh.Line,
					fmt.Sprintf("unexpected section %q",
						formatHeading(dh.Level, dh.Text))))
			}
			docIdx++
		}
		if !found {
			diags = append(diags, makeDiag(f.Path, 1,
				fmt.Sprintf("missing required section %q",
					formatHeading(req.Level, req.Text))))
		}
	}

	// Check remaining doc headings for extra sections when no wildcard
	// allows trailing extras.
	if !allowExtra {
		for docIdx < len(docHeadings) {
			dh := docHeadings[docIdx]
			diags = append(diags, makeDiag(f.Path, dh.Line,
				fmt.Sprintf("unexpected section %q",
					formatHeading(dh.Level, dh.Text))))
			docIdx++
		}
	}

	return diags
}

// matchesSchema checks if a document heading matches a schema heading.
func matchesSchema(req schemaHeading, doc docHeading) bool {
	// Wildcard heading: matches any text at any level.
	if req.Text == "?" {
		return true
	}

	// Check if the schema text contains {field} references.
	if fieldinterp.ContainsField(req.Text) {
		// Split the schema text on {field} patterns, quote-escape
		// the literal parts, and join with .+ to match any value.
		parts := fieldinterp.SplitOnFields(req.Text)
		var pattern strings.Builder
		pattern.WriteString("^")
		for i, part := range parts {
			pattern.WriteString(regexp.QuoteMeta(part))
			if i < len(parts)-1 {
				pattern.WriteString(".+")
			}
		}
		pattern.WriteString("$")
		re, err := regexp.Compile(pattern.String())
		if err != nil {
			return false
		}
		return re.MatchString(doc.Text)
	}

	return doc.Text == req.Text
}

func isSectionWildcard(req schemaHeading) bool {
	return strings.TrimSpace(req.Text) == sectionWildcard
}

// resolveFields replaces {field} placeholders with frontmatter values
// using CUE path resolution for nested access.
func resolveFields(text string, docFM map[string]any) string {
	return fieldinterp.Interpolate(text, docFM)
}

// advanceToMatch advances docIdx to the next heading matching req.
// Returns the matched index (or -1) and the new docIdx.
func advanceToMatch(
	req schemaHeading, docHeadings []docHeading, docIdx int,
) (int, int) {
	for docIdx < len(docHeadings) {
		if matchesSchema(req, docHeadings[docIdx]) {
			return docIdx, docIdx + 1
		}
		docIdx++
	}
	return -1, docIdx
}

// checkSyncPoint checks a single sync point against the document.
func checkSyncPoint(
	f *lint.File, sp syncPoint, req schemaHeading,
	dh docHeading, matchedDoc int, docHeadings []docHeading,
	docFM map[string]any,
) []lint.Diagnostic {
	// Check if the field exists in front matter using CUE path resolution.
	path := fieldinterp.ParseCUEPath(sp.Field)
	if path == nil {
		return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
			fmt.Sprintf("invalid CUE path in sync placeholder: %q", sp.Field))}
	}
	if _, err := fieldinterp.ResolvePath(docFM, path); err != nil {
		return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
			fmt.Sprintf("sync placeholder %q refers to missing or invalid frontmatter path: %v",
				sp.Field, err))}
	}
	if !sp.InBody {
		expected := resolveFields(req.Text, docFM)
		if dh.Text != expected {
			return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
				fmt.Sprintf("heading does not match frontmatter: expected %q (from %s), got %q",
					expected, sp.Field, dh.Text))}
		}
		return nil
	}
	expected := resolveFields(sp.BodyText, docFM)
	return checkBodySync(f, dh, matchedDoc, docHeadings, expected, sp.Field)
}

// checkSync verifies frontmatter-body synchronization.
func checkSync(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
	docFM map[string]any,
) []lint.Diagnostic {
	if len(docFM) == 0 {
		return nil
	}

	var diags []lint.Diagnostic
	docIdx := 0

	for schIdx, req := range sch.Headings {
		if isSectionWildcard(req) {
			continue
		}

		syncs := sch.SyncPoints[schIdx]
		if len(syncs) == 0 {
			_, docIdx = advanceToMatch(req, docHeadings, docIdx)
			continue
		}

		matchedDoc, newIdx := advanceToMatch(req, docHeadings, docIdx)
		docIdx = newIdx
		if matchedDoc < 0 {
			continue
		}

		dh := docHeadings[matchedDoc]
		for _, sp := range syncs {
			diags = append(diags,
				checkSyncPoint(f, sp, req, dh, matchedDoc, docHeadings, docFM)...)
		}
	}

	return diags
}

// checkBodySync checks that expected body text appears under the heading.
// It joins consecutive non-blank lines into paragraphs so that soft-wrapped
// descriptions still match their single-line frontmatter value.
func checkBodySync(
	f *lint.File,
	dh docHeading,
	headingIdx int,
	allHeadings []docHeading,
	expected string,
	field string,
) []lint.Diagnostic {
	// Determine the line range for body content under this heading.
	startLine := dh.Line + 1
	endLine := len(f.Lines)
	if headingIdx+1 < len(allHeadings) {
		endLine = allHeadings[headingIdx+1].Line - 1
	}

	// Check each individual line first (fast path).
	for i := startLine - 1; i < endLine && i < len(f.Lines); i++ {
		line := strings.TrimSpace(string(f.Lines[i]))
		if line == expected {
			return nil
		}
	}

	// Join consecutive non-blank lines into paragraphs and check each.
	var para []string
	for i := startLine - 1; i <= endLine && i <= len(f.Lines); i++ {
		var line string
		if i < endLine && i < len(f.Lines) {
			line = strings.TrimSpace(string(f.Lines[i]))
		}
		if line == "" || i == endLine || i == len(f.Lines) {
			if len(para) > 0 {
				joined := strings.Join(para, " ")
				if joined == expected {
					return nil
				}
				para = para[:0]
			}
			continue
		}
		para = append(para, line)
	}

	return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
		fmt.Sprintf("body does not match frontmatter field %q", field))}
}

func validateCUESchemaSyntax(schema string) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}

	ctx := cuecontext.New()
	v := ctx.CompileString(schema)
	if err := v.Err(); err != nil {
		return fmt.Errorf("invalid schema frontmatter CUE: %w", err)
	}
	return nil
}

func validateFrontMatterCUE(schema string, fm map[string]any) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}

	ctx := cuecontext.New()
	schemaVal := ctx.CompileString(schema)
	if err := schemaVal.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}

	if fm == nil {
		fm = map[string]any{}
	}

	data, err := json.Marshal(fm)
	if err != nil {
		return fmt.Errorf("serialize front matter: %w", err)
	}

	dataVal := ctx.CompileBytes(data)
	if err := dataVal.Err(); err != nil {
		return fmt.Errorf("compile front matter: %w", err)
	}

	merged := schemaVal.Unify(dataVal)
	if err := merged.Validate(cue.Concrete(true)); err != nil {
		return err
	}

	return nil
}

// readDocFrontMatterRaw reads YAML frontmatter from the document.
func readDocFrontMatterRaw(f *lint.File) (map[string]any, []lint.Diagnostic) {
	if len(f.FrontMatter) == 0 {
		return nil, nil
	}

	yamlBytes := extractYAML(f.FrontMatter)
	if yamlBytes == nil {
		return nil, nil
	}

	if err := lint.RejectYAMLAliases(yamlBytes); err != nil {
		return nil, []lint.Diagnostic{makeDiag(f.Path, 1,
			fmt.Sprintf("front matter: %v", err))}
	}
	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return nil, []lint.Diagnostic{makeDiag(f.Path, 1,
			fmt.Sprintf("front matter: invalid YAML: %v", err))}
	}
	return raw, nil
}

// extractYAML extracts the YAML content between --- delimiters.
func extractYAML(fmBlock []byte) []byte {
	s := string(fmBlock)
	s = strings.TrimPrefix(s, "---\n")
	idx := strings.Index(s, "---\n")
	if idx < 0 {
		// Try without trailing newline.
		idx = strings.Index(s, "---")
		if idx < 0 {
			return nil
		}
	}
	return []byte(s[:idx])
}

// findRequireDirectiveLine returns the 1-based line number of the first
// <?require?> PI in the file, or 0 if none is found.
func findRequireDirectiveLine(f *lint.File) int {
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		pi, ok := c.(*lint.ProcessingInstruction)
		if ok && pi.Name == "require" {
			return f.LineOfOffset(pi.Lines().At(0).Start)
		}
	}
	return 0
}

// isSchemaFile checks if the document path is the configured schema.
func isSchemaFile(docPath, schemaPath string) bool {
	docInfo, errDoc := os.Stat(docPath)
	schemaInfo, errSchema := os.Stat(schemaPath)
	if errDoc == nil && errSchema == nil {
		return os.SameFile(docInfo, schemaInfo)
	}

	docAbs, errDocAbs := filepath.Abs(docPath)
	schemaAbs, errSchemaAbs := filepath.Abs(schemaPath)
	if errDocAbs != nil || errSchemaAbs != nil {
		return false
	}
	return filepath.Clean(docAbs) == filepath.Clean(schemaAbs)
}

// formatHeading returns a markdown-style heading string.
func formatHeading(level int, text string) string {
	return strings.Repeat("#", level) + " " + text
}

// checkFilenamePattern checks that the document basename matches the
// schema's filename glob pattern (if configured).
func checkFilenamePattern(
	f *lint.File, sch *parsedSchema,
) []lint.Diagnostic {
	pattern := sch.Config.FilenamePattern
	if pattern == "" {
		return nil
	}
	base := filepath.Base(f.Path)
	matched, err := filepath.Match(pattern, base)
	if err != nil {
		return []lint.Diagnostic{makeDiag(f.Path, 1,
			fmt.Sprintf("invalid filename pattern %q: %v",
				pattern, err))}
	}
	if !matched {
		return []lint.Diagnostic{makeDiag(f.Path, 1,
			fmt.Sprintf(
				"filename %q does not match required pattern %q",
				base, pattern))}
	}
	return nil
}

func makeDiag(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     file,
		Line:     line,
		Column:   1,
		RuleID:   "MDS020",
		RuleName: "required-structure",
		Severity: lint.Error,
		Message:  msg,
	}
}
