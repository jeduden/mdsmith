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
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that a document's heading structure matches a template.
type Rule struct {
	Template string // path to template file
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
		case "template":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("required-structure: template must be a string, got %T", v)
			}
			r.Template = s
		default:
			return fmt.Errorf("required-structure: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"template": "",
	}
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if r.Template == "" {
		return nil
	}

	tmplData, err := os.ReadFile(r.Template)
	if err != nil {
		return []lint.Diagnostic{r.diag(f.Path, 1,
			fmt.Sprintf("cannot read template %q: %v", r.Template, err))}
	}

	tmpl, err := parseTemplate(tmplData)
	if err != nil {
		return []lint.Diagnostic{r.diag(f.Path, 1,
			fmt.Sprintf("invalid template %q: %v", r.Template, err))}
	}

	// Skip the template file itself.
	if isTemplateTargetFile(f.Path, r.Template) {
		return nil
	}

	docHeadings := extractHeadings(f)
	docFMRaw := readDocFrontMatterRaw(f)
	docFM := readDocFrontMatter(f)

	var diags []lint.Diagnostic

	// Check structure: required headings present and in order.
	diags = append(diags, checkStructure(f, tmpl, docHeadings)...)

	// Validate document front matter against template-embedded CUE schema.
	if err := validateFrontMatterCUE(tmpl.Config.FrontMatterCUE, docFMRaw); err != nil {
		diags = append(diags, makeDiag(f.Path, 1,
			fmt.Sprintf("front matter does not satisfy template CUE schema: %v", err)))
	}

	// Check frontmatter-body sync.
	diags = append(diags, checkSync(f, tmpl, docHeadings, docFM)...)

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

// templateConfig holds the parsed template frontmatter.
type templateConfig struct {
	FrontMatterCUE string
}

// templateHeading represents a required heading from the template.
type templateHeading struct {
	Level int
	Text  string // raw text, may contain {{.field}} or ?
}

// parsedTemplate holds the full parsed template.
type parsedTemplate struct {
	Config   templateConfig
	Headings []templateHeading
	// syncPoints maps heading index to list of (field, expected text) pairs
	// for body sync checking.
	SyncPoints map[int][]syncPoint
}

// syncPoint represents a {{.field}} reference in heading text.
type syncPoint struct {
	Field    string
	InBody   bool   // true if in body content, false if in heading
	BodyText string // the full expected body line text with field substituted
}

var fieldPattern = regexp.MustCompile(`\{\{\.(\w+)\}\}`)
var cueIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const sectionWildcard = "..."

// parseTemplateConfig extracts the template configuration from frontmatter.
func parseTemplateConfig(prefix []byte) (templateConfig, error) {
	cfg := templateConfig{}
	if prefix == nil {
		return cfg, nil
	}
	yamlBytes := extractYAML(prefix)
	if hasLegacyFrontMatterCUE(yamlBytes) {
		return cfg, fmt.Errorf(
			"template.front-matter-cue is no longer supported; define frontmatter schema with top-level fields",
		)
	}
	derivedSchema, err := deriveFrontMatterSchemaFromTemplate(yamlBytes)
	if err != nil {
		return cfg, err
	}
	cfg.FrontMatterCUE = derivedSchema
	if err := validateCUESchemaSyntax(cfg.FrontMatterCUE); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func hasLegacyFrontMatterCUE(yamlBytes []byte) bool {
	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return false
	}
	tmplAny, ok := raw["template"]
	if !ok {
		return false
	}
	tmplMap, ok := tmplAny.(map[string]any)
	if !ok {
		return false
	}
	_, found := tmplMap["front-matter-cue"]
	return found
}

func deriveFrontMatterSchemaFromTemplate(yamlBytes []byte) (string, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return "", fmt.Errorf("parsing template frontmatter: %w", err)
	}
	delete(raw, "template")
	if len(raw) == 0 {
		return "", nil
	}

	expr, err := cueExprForMap(raw)
	if err != nil {
		return "", fmt.Errorf("parsing template frontmatter schema: %w", err)
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
		b.WriteString(cueFieldLabel(k))
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

// collectBodySyncPoints scans body content for {{.field}} references and
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
			matches := fieldPattern.FindAllStringSubmatch(trimmed, -1)
			for _, m := range matches {
				syncPoints[currentHeading] = append(
					syncPoints[currentHeading],
					syncPoint{Field: m[1], InBody: true, BodyText: trimmed},
				)
			}
		}
	}
}

// parseTemplate reads template bytes, extracts frontmatter config and
// required headings.
func parseTemplate(data []byte) (*parsedTemplate, error) {
	prefix, content := lint.StripFrontMatter(data)

	cfg, err := parseTemplateConfig(prefix)
	if err != nil {
		return nil, err
	}

	f, err := lint.NewFile("template", content)
	if err != nil {
		return nil, fmt.Errorf("parsing template markdown: %w", err)
	}

	headings := extractHeadings(f)
	tmplHeadings := make([]templateHeading, len(headings))
	syncPoints := make(map[int][]syncPoint)

	for i, h := range headings {
		tmplHeadings[i] = templateHeading{Level: h.Level, Text: h.Text}
		for _, m := range fieldPattern.FindAllStringSubmatch(h.Text, -1) {
			syncPoints[i] = append(syncPoints[i], syncPoint{Field: m[1]})
		}
	}

	collectBodySyncPoints(content, headings, syncPoints)

	return &parsedTemplate{
		Config:     cfg,
		Headings:   tmplHeadings,
		SyncPoints: syncPoints,
	}, nil
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
	tmpl *parsedTemplate,
	docHeadings []docHeading,
) []lint.Diagnostic {
	var diags []lint.Diagnostic

	docIdx := 0
	allowExtra := false
	for _, req := range tmpl.Headings {
		if isSectionWildcard(req) {
			allowExtra = true
			continue
		}

		found := false
		for docIdx < len(docHeadings) {
			dh := docHeadings[docIdx]
			if matchesTemplate(req, dh) {
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

// matchesTemplate checks if a document heading matches a template heading.
func matchesTemplate(req templateHeading, doc docHeading) bool {
	// Wildcard heading: matches any text at any level.
	if req.Text == "?" {
		return true
	}

	// Check if the template text contains {{.field}} references.
	if fieldPattern.MatchString(req.Text) {
		// Split the template text on {{.field}} patterns, quote-escape
		// the literal parts, and join with .+ to match any value.
		parts := fieldPattern.Split(req.Text, -1)
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

func isSectionWildcard(req templateHeading) bool {
	return strings.TrimSpace(req.Text) == sectionWildcard
}

// resolveFields replaces {{.field}} placeholders with frontmatter values.
func resolveFields(text string, docFM map[string]string) string {
	return fieldPattern.ReplaceAllStringFunc(text, func(match string) string {
		field := fieldPattern.FindStringSubmatch(match)[1]
		if v, ok := docFM[field]; ok {
			return v
		}
		return match
	})
}

// advanceToMatch advances docIdx to the next heading matching req.
// Returns the matched index (or -1) and the new docIdx.
func advanceToMatch(
	req templateHeading, docHeadings []docHeading, docIdx int,
) (int, int) {
	for docIdx < len(docHeadings) {
		if matchesTemplate(req, docHeadings[docIdx]) {
			return docIdx, docIdx + 1
		}
		docIdx++
	}
	return -1, docIdx
}

// checkSyncPoint checks a single sync point against the document.
func checkSyncPoint(
	f *lint.File, sp syncPoint, req templateHeading,
	dh docHeading, matchedDoc int, docHeadings []docHeading,
	docFM map[string]string,
) []lint.Diagnostic {
	if _, ok := docFM[sp.Field]; !ok {
		return nil
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
	tmpl *parsedTemplate,
	docHeadings []docHeading,
	docFM map[string]string,
) []lint.Diagnostic {
	if len(docFM) == 0 {
		return nil
	}

	var diags []lint.Diagnostic
	docIdx := 0

	for tmplIdx, req := range tmpl.Headings {
		if isSectionWildcard(req) {
			continue
		}

		syncs := tmpl.SyncPoints[tmplIdx]
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
		return fmt.Errorf("invalid template frontmatter schema: %w", err)
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

func stringifyFrontMatter(raw map[string]any) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// readDocFrontMatter reads YAML frontmatter from the document and returns a
// stringified view used by sync checks.
func readDocFrontMatter(f *lint.File) map[string]string {
	return stringifyFrontMatter(readDocFrontMatterRaw(f))
}

// readDocFrontMatterRaw reads YAML frontmatter from the document.
func readDocFrontMatterRaw(f *lint.File) map[string]any {
	if len(f.FrontMatter) == 0 {
		return nil
	}

	yamlBytes := extractYAML(f.FrontMatter)
	if yamlBytes == nil {
		return nil
	}

	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return nil
	}
	return raw
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

// isTemplateTargetFile checks if the document path is the configured template.
func isTemplateTargetFile(docPath, templatePath string) bool {
	docInfo, errDoc := os.Stat(docPath)
	tmplInfo, errTmpl := os.Stat(templatePath)
	if errDoc == nil && errTmpl == nil {
		return os.SameFile(docInfo, tmplInfo)
	}

	docAbs, errDocAbs := filepath.Abs(docPath)
	tmplAbs, errTmplAbs := filepath.Abs(templatePath)
	if errDocAbs != nil || errTmplAbs != nil {
		return false
	}
	return filepath.Clean(docAbs) == filepath.Clean(tmplAbs)
}

// formatHeading returns a markdown-style heading string.
func formatHeading(level int, text string) string {
	return strings.Repeat("#", level) + " " + text
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
