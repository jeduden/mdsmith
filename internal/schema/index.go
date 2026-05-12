package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
)

// IndexHeading is one entry in the flat heading list emitted by the
// "headings" include.
type IndexHeading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Slug  string `json:"slug"`
	Line  int    `json:"line"`
}

// BuildIndex computes the JSON index document the IndexSpec asks for
// and returns its serialised bytes. The returned bytes are
// pretty-printed with two-space indentation so the file is reviewable
// when diffed.
func BuildIndex(f *lint.File, sch *Schema) ([]byte, error) {
	if sch == nil || sch.Index == nil {
		return nil, nil
	}
	doc := map[string]any{}
	for _, key := range sch.Index.Include {
		switch key {
		case IndexIncludeStepMap:
			doc[key] = buildStepMap(f)
		case IndexIncludeCrossRefs:
			doc[key] = buildCrossRefGraph(f, sch)
		case IndexIncludeWordCounts:
			doc[key] = buildWordCounts(f)
		case IndexIncludeHeadingsFlat:
			doc[key] = buildFlatHeadings(f)
		default:
			return nil, fmt.Errorf("schema.index.include: unknown entry %q", key)
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}

// WriteIndex writes the JSON index produced by BuildIndex next to
// the source file. Output paths are resolved relative to the source
// file's directory; absolute paths and parent-traversal segments are
// rejected so a schema cannot trick fix into writing outside the
// project.
func WriteIndex(f *lint.File, sch *Schema) error {
	target, data, err := resolveIndexWrite(f, sch)
	if err != nil || data == nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

// resolveIndexWrite returns the absolute output path and the bytes
// that would be written for this file. data is nil when the schema
// declares no index. Path validation matches WriteIndex so both
// call sites surface the same errors.
func resolveIndexWrite(f *lint.File, sch *Schema) (string, []byte, error) {
	if sch == nil || sch.Index == nil {
		return "", nil, nil
	}
	out := sch.Index.Output
	if filepath.IsAbs(out) {
		return "", nil, fmt.Errorf("schema.index.output %q must be relative", out)
	}
	for _, elem := range strings.Split(filepath.ToSlash(out), "/") {
		if elem == ".." {
			return "", nil, fmt.Errorf(
				"schema.index.output %q must not contain \"..\" traversal", out)
		}
	}
	data, err := BuildIndex(f, sch)
	if err != nil {
		return "", nil, err
	}
	if data == nil {
		return "", nil, nil
	}
	data = append(data, '\n')
	dir := filepath.Dir(f.Path)
	target := filepath.Clean(filepath.Join(dir, out))
	return target, data, nil
}

// ValidateIndex compares the on-disk index file (if any) against the
// bytes BuildIndex would emit. When they differ — or when the file
// is missing — a single diagnostic asks the user to run
// `mdsmith fix` so the artefact stays in sync. The diagnostic is
// what triggers `mdsmith fix` to call the rule's Fix() pass, which
// in turn writes the file. `mdsmith check` still respects the
// read-only contract: it never touches the file.
func ValidateIndex(f *lint.File, sch *Schema, mkDiag MakeDiag) []lint.Diagnostic {
	target, want, err := resolveIndexWrite(f, sch)
	if err != nil {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf("index: %v", err))}
	}
	if want == nil {
		return nil
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q is missing; run `mdsmith fix`",
				sch.Index.Output))}
	}
	if string(got) != string(want) {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q is out of date; run `mdsmith fix`",
				sch.Index.Output))}
	}
	return nil
}

// buildFlatHeadings returns every heading in document order with its
// level, plain text, slug, and 1-based line.
func buildFlatHeadings(f *lint.File) []IndexHeading {
	var out []IndexHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := mdtext.ExtractPlainText(h, f.Source)
		line := 1
		if h.Lines().Len() > 0 {
			line = f.LineOfOffset(h.Lines().At(0).Start)
		}
		out = append(out, IndexHeading{
			Level: h.Level,
			Text:  text,
			Slug:  mdtext.Slugify(text),
			Line:  line,
		})
		return ast.WalkContinue, nil
	})
	if out == nil {
		out = []IndexHeading{}
	}
	return out
}

// buildStepMap returns a map of section slug → list of immediate
// child slugs. The map is keyed by the parent's slug for stable JSON
// output regardless of doc order.
func buildStepMap(f *lint.File) map[string][]string {
	heads := buildFlatHeadings(f)
	out := map[string][]string{}
	// Use a stack of (slug, level) for the current path.
	type frame struct {
		slug  string
		level int
	}
	var stack []frame
	for _, h := range heads {
		for len(stack) > 0 && stack[len(stack)-1].level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1].slug
			out[parent] = append(out[parent], h.Slug)
		}
		stack = append(stack, frame{slug: h.Slug, level: h.Level})
	}
	return out
}

// buildCrossRefGraph maps each cross-reference match found in the
// document to its target slug. Unresolved references are still
// emitted (target slug may be empty) so downstream tools see what
// links exist regardless of validation outcome.
func buildCrossRefGraph(f *lint.File, sch *Schema) map[string]string {
	out := map[string]string{}
	if len(sch.CrossReferences) == 0 {
		return out
	}
	texts := collectTextNodes(f)
	for _, cr := range sch.CrossReferences {
		re, err := regexp.Compile(cr.Pattern)
		if err != nil {
			continue
		}
		var skipRE *regexp.Regexp
		if cr.SkipLinesMatching != "" {
			skipRE, _ = regexp.Compile(cr.SkipLinesMatching)
		}
		groupNames := re.SubexpNames()
		for _, tn := range texts {
			if skipRE != nil && lineMatches(f, tn.Line, skipRE) {
				continue
			}
			for _, m := range re.FindAllStringSubmatch(tn.Text, -1) {
				target, err := fillTemplate(cr.MustMatch, m, groupNames)
				if err != nil {
					continue
				}
				out[m[0]] = mdtext.Slugify(target)
			}
		}
	}
	return out
}

// buildWordCounts maps each heading slug to the word count of the
// body text immediately under that heading — up to but excluding
// the next heading at any level. Sub-section text is attributed to
// that subsection's slug, not the parent's, so summing along the
// step-map child list gives the recursive total when callers want
// it.
func buildWordCounts(f *lint.File) map[string]int {
	heads := buildFlatHeadings(f)
	out := map[string]int{}
	for i, h := range heads {
		startLine := h.Line + 1
		endLine := len(f.Lines) + 1
		if i+1 < len(heads) {
			endLine = heads[i+1].Line
		}
		count := 0
		for ln := startLine; ln < endLine && ln-1 < len(f.Lines); ln++ {
			count += len(strings.Fields(string(f.Lines[ln-1])))
		}
		out[h.Slug] = count
	}
	return out
}
