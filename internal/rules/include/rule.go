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

func (r *Rule) generateIncludeContent(
	f *lint.File, filePath string, line int,
	params map[string]string,
) (string, []lint.Diagnostic) {
	file := params["file"]

	// Normalize to slash-separated paths for the path package and fs.FS.
	filePath = filepath.ToSlash(filePath)

	// Resolve file relative to the including file's directory.
	// Use RootFS (project root) when available so that paths
	// with ".." segments work across directories.
	resolvedFile := path.Clean(path.Join(path.Dir(filePath), file))
	readFS := f.FS
	readPath := path.Clean(file)
	if f.RootFS != nil {
		// Reject resolved paths that escape the project root.
		if strings.HasPrefix(resolvedFile, "..") {
			return "", []lint.Diagnostic{makeDiag(filePath, line,
				`include file path escapes project root`)}
		}
		readFS = f.RootFS
		readPath = resolvedFile
	} else if containsDotDotElement(file) {
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			`include file path contains ".." but project root is not configured`)}
	}

	// Check max include depth.
	if r.visited != nil && len(r.chain) > maxIncludeDepth {
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("include depth exceeds maximum (%d)", maxIncludeDepth))}
	}

	// Check for cyclic include.
	if r.visited != nil && r.visited[resolvedFile] {
		chain := make([]string, len(r.chain))
		copy(chain, r.chain)
		chain = append(chain, resolvedFile)
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("cyclic include: %s", strings.Join(chain, " -> ")))}
	}

	data, err := fs.ReadFile(readFS, readPath)
	if err != nil {
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("include file %q not found: %v", file, err))}
	}

	// Track this file and scan for nested include cycles.
	if r.visited != nil {
		r.visited[resolvedFile] = true
		r.chain = append(r.chain, resolvedFile)
		if diags := r.scanForCycles(readFS, data, resolvedFile, filePath, line); len(diags) > 0 {
			return "", diags
		}
		delete(r.visited, resolvedFile)
		r.chain = r.chain[:len(r.chain)-1]
	}

	content := data

	// strip-frontmatter defaults to true.
	stripFM := true
	if sfm, ok := params["strip-frontmatter"]; ok && sfm == "false" {
		stripFM = false
	}

	if stripFM {
		_, stripped := lint.StripFrontMatter(content)
		content = stripped
	}

	text := string(content)

	// Trim leading blank line (common after stripping frontmatter).
	text = strings.TrimLeft(text, "\n")

	// Rewrite relative links so they resolve from the including file's
	// directory. The file param is relative to f.FS (the including file's
	// directory), so join with filePath's directory to get a repo-root-
	// relative path matching filePath's coordinate system.
	includedPath := path.Join(path.Dir(filePath), file)
	text = adjustLinks(text, includedPath, filePath)

	// Shift headings when heading-level: "absolute" is set.
	if params["heading-level"] == "absolute" {
		parentLevel := findParentHeadingLevel(f, line)
		text = adjustHeadings(text, parentLevel)
	}

	// Wrap in code fence if requested.
	if wrap, ok := params["wrap"]; ok {
		fence := strings.Repeat("`", minFenceLen(text))
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text = "\n" + fence + wrap + "\n" + text + fence + "\n\n"
	}

	return gensection.EnsureTrailingNewline(text), nil
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

// scanForCycles parses the included file for nested include directives and
// checks for cycles in the include chain. It uses already-read data to
// avoid double reads for the first level.
func (r *Rule) scanForCycles(
	readFS fs.FS, data []byte, currentPath, originFile string, originLine int,
) []lint.Diagnostic {
	_, content := lint.StripFrontMatter(data)
	f, err := lint.NewFile(currentPath, content)
	if err != nil {
		return nil
	}

	pairs, _ := gensection.FindMarkerPairs(f, "include", "MDS021", "include")
	for _, mp := range pairs {
		dir, diags := gensection.ParseDirective(currentPath, mp, "MDS021", "include")
		if dir == nil || len(diags) > 0 {
			continue
		}
		file := dir.Params["file"]
		if file == "" {
			continue
		}

		resolved := path.Clean(path.Join(path.Dir(currentPath), file))

		// Check depth.
		if len(r.chain) > maxIncludeDepth {
			return []lint.Diagnostic{makeDiag(originFile, originLine,
				fmt.Sprintf("include depth exceeds maximum (%d)", maxIncludeDepth))}
		}

		// Check cycle.
		if r.visited[resolved] {
			chain := make([]string, len(r.chain))
			copy(chain, r.chain)
			chain = append(chain, resolved)
			return []lint.Diagnostic{makeDiag(originFile, originLine,
				fmt.Sprintf("cyclic include: %s", strings.Join(chain, " -> ")))}
		}

		// Recurse into nested includes.
		r.visited[resolved] = true
		r.chain = append(r.chain, resolved)
		nested, readErr := fs.ReadFile(readFS, resolved)
		if readErr == nil {
			if diags := r.scanForCycles(readFS, nested, resolved, originFile, originLine); len(diags) > 0 {
				return diags
			}
		}
		delete(r.visited, resolved)
		r.chain = r.chain[:len(r.chain)-1]
	}

	return nil
}

var _ rule.FixableRule = (*Rule)(nil)
