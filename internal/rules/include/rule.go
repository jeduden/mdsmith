package include

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// maxIncludeDepth is the maximum nesting depth for include chains.
const maxIncludeDepth = 10

// Rule checks that include sections contain the correct file content.
type Rule struct {
	engine  *gensection.Engine
	visited map[string]bool // files in current include chain
	chain   []string        // ordered chain for cycle diagnostics
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS021" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "include" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// RuleID implements gensection.Directive.
func (r *Rule) RuleID() string { return "MDS021" }

// RuleName implements gensection.Directive.
func (r *Rule) RuleName() string { return "include" }

func (r *Rule) getEngine() *gensection.Engine {
	if r.engine == nil {
		r.engine = gensection.NewEngine(r)
	}
	return r.engine
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.FS == nil {
		return nil
	}
	p := filepath.ToSlash(f.Path)
	r.visited = map[string]bool{p: true}
	r.chain = []string{p}
	defer func() { r.visited = nil; r.chain = nil }()
	return r.getEngine().Check(f)
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	if f.FS == nil {
		return f.Source
	}
	p := filepath.ToSlash(f.Path)
	r.visited = map[string]bool{p: true}
	r.chain = []string{p}
	defer func() { r.visited = nil; r.chain = nil }()
	return r.getEngine().Fix(f)
}

// Validate implements gensection.Directive.
func (r *Rule) Validate(
	filePath string, line int,
	params map[string]string,
	columns map[string]gensection.ColumnConfig,
) []lint.Diagnostic {
	return validateIncludeDirective(filePath, line, params)
}

// Generate implements gensection.Directive.
func (r *Rule) Generate(
	f *lint.File, filePath string, line int,
	params map[string]string,
	columns map[string]gensection.ColumnConfig,
) (string, []lint.Diagnostic) {
	return r.generateIncludeContent(f, filePath, line, params)
}

func validateIncludeDirective(
	filePath string, line int,
	params map[string]string,
) []lint.Diagnostic {
	file, hasFile := params["file"]
	if !hasFile || strings.TrimSpace(file) == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`include directive missing required "file" parameter`)}
	}

	if filepath.IsAbs(file) {
		return []lint.Diagnostic{makeDiag(filePath, line,
			"include directive has absolute file path")}
	}

	// Validate wrap parameter if present.
	if wrap, ok := params["wrap"]; ok && strings.TrimSpace(wrap) == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`include directive has empty "wrap" value`)}
	}

	// Validate strip-frontmatter parameter if present.
	if sfm, ok := params["strip-frontmatter"]; ok {
		if sfm != "true" && sfm != "false" {
			return []lint.Diagnostic{makeDiag(filePath, line,
				`include directive "strip-frontmatter" must be "true" or "false"`)}
		}
	}

	// Validate heading-level parameter if present.
	if hl, ok := params["heading-level"]; ok {
		if hl != "absolute" {
			return []lint.Diagnostic{makeDiag(filePath, line,
				`include directive "heading-level" must be "absolute"`)}
		}
	}

	return nil
}

// resolveIncludePath resolves the included file path and returns the
// filesystem and path to read from, or a diagnostic on error.
func resolveIncludePath(
	f *lint.File, filePath, file string, line int,
) (fs.FS, string, string, []lint.Diagnostic) {
	resolvedFile := path.Clean(path.Join(path.Dir(filePath), file))
	readFS := f.FS
	readPath := path.Clean(file)
	if f.RootFS != nil {
		if strings.HasPrefix(resolvedFile, "..") {
			return nil, "", "", []lint.Diagnostic{makeDiag(filePath, line,
				`include file path escapes project root`)}
		}
		readFS = f.RootFS
		readPath = resolvedFile
	} else if containsDotDotElement(file) {
		return nil, "", "", []lint.Diagnostic{makeDiag(filePath, line,
			`include file path contains ".." but project root is not configured`)}
	}
	return readFS, readPath, resolvedFile, nil
}

// checkCycleOrDepth returns a diagnostic if the resolved file would
// create a cycle or exceed the max include depth.
func (r *Rule) checkCycleOrDepth(
	resolvedFile, filePath string, line int,
) []lint.Diagnostic {
	if r.visited == nil {
		return nil
	}
	if len(r.chain) > maxIncludeDepth {
		return []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("include depth exceeds maximum (%d)", maxIncludeDepth))}
	}
	if r.visited[resolvedFile] {
		chain := make([]string, len(r.chain))
		copy(chain, r.chain)
		chain = append(chain, resolvedFile)
		return []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("cyclic include: %s", strings.Join(chain, " -> ")))}
	}
	return nil
}

func (r *Rule) generateIncludeContent(
	f *lint.File, filePath string, line int,
	params map[string]string,
) (string, []lint.Diagnostic) {
	file := params["file"]
	filePath = filepath.ToSlash(filePath)

	readFS, readPath, resolvedFile, diags := resolveIncludePath(f, filePath, file, line)
	if len(diags) > 0 {
		return "", diags
	}

	if diags := r.checkCycleOrDepth(resolvedFile, filePath, line); len(diags) > 0 {
		return "", diags
	}

	data, err := fs.ReadFile(readFS, readPath)
	if err != nil {
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("include file %q not found: %v", file, err))}
	}

	// Track this file and recursively expand nested includes.
	if r.visited != nil {
		r.visited[resolvedFile] = true
		r.chain = append(r.chain, resolvedFile)
		defer func() {
			delete(r.visited, resolvedFile)
			r.chain = r.chain[:len(r.chain)-1]
		}()
		expanded, expandDiags := r.expandNestedIncludes(readFS, data, resolvedFile)
		if len(expandDiags) > 0 {
			// Remap diagnostics to this include directive so the
			// lint pipeline uses the correct file for context.
			for i := range expandDiags {
				d := &expandDiags[i]
				d.Message = fmt.Sprintf(
					"%s:%d: %s", d.File, d.Line, d.Message)
				d.File = filePath
				d.Line = line
				d.Column = 1
			}
			return "", expandDiags
		}
		data = expanded
	}

	text := processIncludedContent(data, params, f, filePath, file, line)
	return gensection.EnsureTrailingNewline(text), nil
}

// processIncludedContent strips frontmatter, adjusts links/headings,
// and optionally wraps in a code fence.
func processIncludedContent(
	data []byte, params map[string]string,
	f *lint.File, filePath, file string, line int,
) string {
	content := data
	stripFM := true
	if sfm, ok := params["strip-frontmatter"]; ok && sfm == "false" {
		stripFM = false
	}
	if stripFM {
		_, stripped := lint.StripFrontMatter(content)
		content = stripped
	}

	text := strings.TrimLeft(string(content), "\n")
	includedPath := path.Join(path.Dir(filePath), file)
	text = adjustLinks(text, includedPath, filePath)

	// Inject source-dir into processing instructions so downstream
	// directives (e.g. catalog) resolve globs relative to the included
	// file's directory, not the including file's directory (#133).
	// Skip when content will be wrapped in a code fence, since wrapped
	// content is displayed as code and PIs are not parsed.
	if _, wrapped := params["wrap"]; !wrapped {
		includedDir := path.Dir(includedPath)
		includerDir := path.Dir(filePath)
		if includedDir != includerDir {
			text = injectSourceDir(text, includedDir)
		}
	}

	if params["heading-level"] == "absolute" {
		text = adjustHeadings(text, findParentHeadingLevel(f, line))
	}
	if wrap, ok := params["wrap"]; ok {
		fence := strings.Repeat("`", minFenceLen(text))
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text = "\n" + fence + wrap + "\n" + text + fence + "\n\n"
	}
	return text
}

// findParentHeadingLevel returns the level of the most recent heading
// before the given 1-based line in the file's AST. Returns 0 if the
// marker is at the document root (no heading precedes it).
func findParentHeadingLevel(f *lint.File, markerLine int) int {
	parentLevel := 0
	for child := f.AST.FirstChild(); child != nil; child = child.NextSibling() {
		heading, ok := child.(*ast.Heading)
		if !ok {
			continue
		}
		if heading.Lines().Len() == 0 {
			continue
		}
		headingLine := f.LineOfOffset(heading.Lines().At(0).Start)
		if headingLine >= markerLine {
			break
		}
		parentLevel = heading.Level
	}
	return parentLevel
}

func makeDiag(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     file,
		Line:     line,
		Column:   1,
		RuleID:   "MDS021",
		RuleName: "include",
		Severity: lint.Error,
		Message:  msg,
	}
}

// minFenceLen returns the minimum backtick fence length needed to safely
// wrap text without conflicting with backtick runs inside the content.
func minFenceLen(text string) int {
	n := 3
	for _, line := range strings.Split(text, "\n") {
		run := 0
		for _, c := range line {
			if c == '`' {
				run++
				if run >= n {
					n = run + 1
				}
			} else {
				run = 0
			}
		}
	}
	return n
}

// containsDotDotElement reports whether the slash-separated path contains
// a ".." path element. It does not match filenames like "foo..bar.md".
func containsDotDotElement(p string) bool {
	for _, elem := range strings.Split(p, "/") {
		if elem == ".." {
			return true
		}
	}
	return false
}

// expandNestedIncludes parses the included file for nested include
// directives and recursively expands them, so that a single fix pass
// produces the fully flattened output regardless of file order.
func (r *Rule) expandNestedIncludes(
	readFS fs.FS, data []byte, resolvedFile string,
) ([]byte, []lint.Diagnostic) {
	// Parse the full data including frontmatter so that marker pair
	// line numbers match the real file. Goldmark treats frontmatter
	// delimiters as thematic breaks, which FindMarkerPairs skips.
	// processIncludedContent handles frontmatter stripping later.
	f, err := lint.NewFile(resolvedFile, data)
	if err != nil {
		return data, nil
	}
	f.FS = readFS
	// RootFS must be set so resolveIncludePath uses root-relative
	// readPath. Without it, readPath is the raw file parameter,
	// which only works when FS is scoped to the including file's
	// own directory — not the case here since readFS is scoped to
	// the project root. The ".." escape check still applies because
	// resolvedFile is computed via path.Join before the prefix test.
	f.RootFS = readFS

	pairs, _ := gensection.FindMarkerPairs(f, "include", "MDS021", "include")
	if len(pairs) == 0 {
		return data, nil
	}

	// Work backwards to preserve line numbers.
	for i := len(pairs) - 1; i >= 0; i-- {
		mp := pairs[i]
		dir, pdiags := gensection.ParseDirective(resolvedFile, mp, "MDS021", "include")
		if dir == nil || len(pdiags) > 0 {
			return nil, pdiags
		}

		valDiags := validateIncludeDirective(resolvedFile, mp.StartLine, dir.Params)
		if len(valDiags) > 0 {
			return nil, valDiags
		}

		generated, genDiags := r.generateIncludeContent(
			f, resolvedFile, mp.StartLine, dir.Params,
		)
		if len(genDiags) > 0 {
			return nil, genDiags
		}

		f.Source = gensection.ReplaceContent(f, mp, generated)
		f.Lines = gensection.SplitLines(f.Source)
	}

	return f.Source, nil
}

// injectSourceDir adds a source-dir parameter to processing instruction
// start markers in text. It parses the text as Markdown to avoid modifying
// PIs inside code blocks.
func injectSourceDir(text, sourceDir string) string {
	parsed, err := lint.NewFile("", []byte(text))
	if err != nil {
		return text
	}

	// Collect byte offsets where the source-dir line should be inserted.
	type injection struct {
		offset int
	}
	var injections []injection

	for n := parsed.AST.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*lint.ProcessingInstruction)
		if !ok {
			continue
		}
		// Skip end markers (<?/name?>).
		if strings.HasPrefix(pi.Name, "/") {
			continue
		}
		firstSeg := pi.Lines().At(0)
		// Skip single-line PIs (<?name ...?>).
		if pi.HasClosure() && pi.ClosureLine.Start == firstSeg.Start {
			continue
		}
		// Skip if source-dir is already present in the PI body.
		already := false
		for i := 1; i < pi.Lines().Len(); i++ {
			seg := pi.Lines().At(i)
			if strings.Contains(string(seg.Value([]byte(text))), "source-dir:") {
				already = true
				break
			}
		}
		if already {
			continue
		}

		injections = append(injections, injection{offset: firstSeg.Stop})
	}

	if len(injections) == 0 {
		return text
	}

	sdLine := fmt.Sprintf("source-dir: \"%s\"\n", sourceDir)
	var b strings.Builder
	prev := 0
	for _, inj := range injections {
		b.WriteString(text[prev:inj.offset])
		b.WriteString(sdLine)
		prev = inj.offset
	}
	b.WriteString(text[prev:])
	return b.String()
}

var _ rule.FixableRule = (*Rule)(nil)
