package schema

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/yuin/goldmark/ast"
)

// FileReader resolves a schema path or include reference to the
// underlying bytes. RootFS, when set, is used to constrain reads to
// the project root; otherwise reads go through os.ReadFile.
type FileReader struct {
	RootFS   fs.FS
	RootDir  string
	MaxBytes int64
}

// ParseFile loads the proto.md schema at path through r and returns
// the parsed Schema. It expands <?include?> directives, extracts
// <?require?> filename constraints, and normalises the flat
// heading-template into a Scope tree (each H2 becomes a top-level
// Scope, each H3 nests beneath the previous H2, and so on).
//
// File-based schemas default to Closed=true at the root so the
// historical heading-template behaviour (extras flagged) survives the
// migration. Inline schemas opt back to open scopes per plan 146.
//
// Plan 156 maps the proto.md heading-row tokens to the new Matcher
// shape: `## ?` → `{regex: '.+'}`, `## ...` → `{regex: '.+',
// repeat: {min: 0}}`, `## Step {n}` → `{regex: 'Step \#(digits)'}`,
// `## {id}` → `{regex: '\#(fmvar(id))'}`. The `<?require?>`
// directive in proto.md bodies is unchanged.
//
// Note: MDS020's file-schema path (internal/rules/requiredstructure)
// still routes through its legacy `parseSchema` / `parsedSchema`
// pipeline, not this `ParseFile`. The new shape parses correctly
// here and is exercised by the schema-package tests, but proto.md
// authors won't benefit from the new Matcher / fmvar / digits
// semantics until that path is migrated (a follow-up plan tracks
// the cutover so the `{field}` heading/body sync feature survives
// the move).
func ParseFile(r *FileReader, path string) (*Schema, error) {
	if r == nil {
		r = &FileReader{}
	}
	data, err := r.readPath(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read schema %q: %w", path, err)
	}
	return parseFileBytes(r, path, data, map[string]bool{filepath.Clean(path): true}, []string{filepath.Clean(path)})
}

const maxIncludeDepth = 10

func parseFileBytes(
	r *FileReader, path string, data []byte,
	visited map[string]bool, chain []string,
) (*Schema, error) {
	prefix, body := lint.StripFrontMatter(data)

	sch := &Schema{Source: path, Closed: true}

	if err := parseFileFrontmatter(prefix, sch); err != nil {
		return nil, err
	}

	f, err := lint.NewFile(path, body)
	if err != nil {
		return nil, fmt.Errorf("parsing schema markdown %q: %w", path, err)
	}

	fp, err := extractRequireFilename(f)
	if err != nil {
		return nil, err
	}
	if fp != "" {
		sch.Filename = fp
	}

	headings, fp2, err := collectFileHeadings(f, r, path, visited, chain)
	if err != nil {
		return nil, err
	}
	if sch.Filename == "" && fp2 != "" {
		sch.Filename = fp2
	}

	sch.Sections, sch.RootLevel, err = headingsToScopes(headings)
	if err != nil {
		return nil, err
	}
	return sch, nil
}

// FileHeading is a heading collected from a schema markdown file.
type FileHeading struct {
	Level int
	Text  string
}

func parseFileFrontmatter(prefix []byte, sch *Schema) error {
	if prefix == nil {
		return nil
	}
	body := stripDelimiters(prefix)
	if len(body) == 0 {
		return nil
	}
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(body, &raw); err != nil {
		return fmt.Errorf("parsing schema frontmatter: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	sch.Frontmatter = make(map[string]string, len(raw))
	for k, v := range raw {
		expr, err := frontmatterExpr(v)
		if err != nil {
			return fmt.Errorf("schema frontmatter %q: %w", k, err)
		}
		sch.Frontmatter[k] = expr
	}
	// Capture per-key source lines so MDS020 diagnostics can point
	// the reader at the exact constraint. yaml.Node line numbers
	// are 1-based within the stripped YAML body; the body starts
	// on the schema file's line 2 (line 1 is the opening "---"
	// fence), so we add 1 to translate into file-relative lines.
	node, err := yamlutil.UnmarshalNodeSafe(body)
	if err == nil {
		sch.FrontmatterLines = yamlutil.TopLevelMappingLines(&node, 1)
	}
	return nil
}

// stripDelimiters returns the YAML body between a front-matter
// block's "---\n" delimiters. The caller (parseFileFrontmatter)
// receives prefix bytes produced by lint.StripFrontMatter, which
// guarantees the block is bracketed by "---\n" on both ends.
//
// The closing fence is removed via TrimSuffix rather than a
// strings.Index scan: searching for the first "---\n" anywhere
// in the body would truncate the YAML early if a block-scalar
// value (e.g. `notes: |`) legitimately contained the same
// sequence on one of its indented lines.
func stripDelimiters(fm []byte) []byte {
	s := bytes.TrimPrefix(fm, []byte("---\n"))
	return bytes.TrimSuffix(s, []byte("---\n"))
}

// extractRequireFilename walks the schema AST for a <?require?> PI
// and parses its YAML body to extract the filename constraint.
func extractRequireFilename(f *lint.File) (string, error) {
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		pi, ok := c.(*lint.ProcessingInstruction)
		if !ok || pi.Name != "require" {
			continue
		}
		body, err := piYAMLBody(pi, f.Source, "require")
		if err != nil {
			return "", err
		}
		if body == "" {
			continue
		}
		var params map[string]string
		if err := yamlutil.UnmarshalSafe([]byte(body), &params); err != nil {
			return "", fmt.Errorf("invalid <?require?> directive: %w", err)
		}
		return params["filename"], nil
	}
	return "", nil
}

func piYAMLBody(pi *lint.ProcessingInstruction, source []byte, name string) (string, error) {
	lines := pi.Lines()
	if lines.Len() == 1 {
		seg := lines.At(0)
		line := strings.TrimSpace(string(seg.Value(source)))
		line = strings.TrimPrefix(line, "<?"+name)
		if idx := strings.Index(line, "?>"); idx >= 0 {
			line = line[:idx]
		}
		return strings.TrimSpace(line), nil
	}
	var b strings.Builder
	for i := 1; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String(), nil
}

// collectFileHeadings walks the schema AST, collecting heading entries
// and expanding <?include?> PIs by splicing the included file's
// headings in place.
func collectFileHeadings(
	f *lint.File, r *FileReader, path string,
	visited map[string]bool, chain []string,
) ([]FileHeading, string, error) {
	var heads []FileHeading
	var fp string
	err := ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			text := headingText(node, f.Source)
			heads = append(heads, FileHeading{Level: node.Level, Text: text})
		case *lint.ProcessingInstruction:
			if node.Name != "include" {
				return ast.WalkContinue, nil
			}
			frag, fpInc, err := expandInclude(node, f.Source, r, path, visited, chain)
			if err != nil {
				return ast.WalkStop, err
			}
			if fpInc != "" && fp == "" {
				fp = fpInc
			}
			heads = append(heads, frag...)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, "", err
	}
	return heads, fp, nil
}

func expandInclude(
	pi *lint.ProcessingInstruction, source []byte,
	r *FileReader, schemaPath string, visited map[string]bool, chain []string,
) ([]FileHeading, string, error) {
	included, err := resolveIncludePath(pi, source, schemaPath)
	if err != nil {
		return nil, "", err
	}

	if len(chain) > maxIncludeDepth {
		return nil, "", fmt.Errorf(
			"schema include depth exceeds maximum (%d)", maxIncludeDepth)
	}
	if visited[included] {
		chainCopy := append([]string(nil), chain...)
		chainCopy = append(chainCopy, included)
		return nil, "", fmt.Errorf(
			"cyclic include: %s", strings.Join(chainCopy, " -> "))
	}

	data, err := r.readPath(included)
	if err != nil {
		return nil, "", fmt.Errorf(
			"cannot read schema include file %q: %w", included, err)
	}

	_, body := lint.StripFrontMatter(data)
	fragFile, err := lint.NewFile(included, body)
	if err != nil {
		return nil, "", fmt.Errorf(
			"parsing schema include %q: %w", included, err)
	}

	fp, err := extractRequireFilename(fragFile)
	if err != nil {
		return nil, "", err
	}

	visited[included] = true
	nextChain := append([]string(nil), chain...)
	nextChain = append(nextChain, included)
	frag, fpFrag, err := collectFileHeadings(fragFile, r, included, visited, nextChain)
	delete(visited, included)
	if err != nil {
		return nil, "", err
	}
	if fpFrag != "" && fp == "" {
		fp = fpFrag
	}

	return frag, fp, nil
}

func resolveIncludePath(
	pi *lint.ProcessingInstruction, source []byte, schemaPath string,
) (string, error) {
	body, err := piYAMLBody(pi, source, pi.Name)
	if err != nil {
		return "", fmt.Errorf("parsing include processing instruction: %w", err)
	}
	if body == "" {
		return "", fmt.Errorf(
			"include processing instruction missing required 'file' attribute")
	}
	var params map[string]string
	if err := yamlutil.UnmarshalSafe([]byte(body), &params); err != nil {
		return "", fmt.Errorf("invalid include directive YAML: %w", err)
	}
	fileParam := strings.TrimSpace(params["file"])
	if fileParam == "" {
		return "", fmt.Errorf(
			"include processing instruction missing required 'file' attribute")
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

// readPath reads through RootFS when configured, falling back to the
// OS filesystem. The behaviour mirrors the legacy
// requiredstructure.readSchemaFile helper so existing fixtures keep
// resolving paths the same way. A zero MaxBytes is normalised to
// lint.DefaultMaxInputBytes so a default-constructed FileReader
// does not silently read unbounded schema/include files.
func (r *FileReader) readPath(path string) ([]byte, error) {
	maxBytes := r.MaxBytes
	if maxBytes == 0 {
		maxBytes = lint.DefaultMaxInputBytes
	}
	if r.RootFS != nil {
		if filepath.IsAbs(path) {
			return nil, fmt.Errorf("absolute schema path not allowed")
		}
		clean := filepath.ToSlash(filepath.Clean(path))
		clean = strings.TrimPrefix(clean, "./")
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("schema path %q escapes project root", path)
		}
		return lint.ReadFSFileLimited(r.RootFS, clean, maxBytes)
	}
	return lint.ReadFileLimited(path, maxBytes)
}

// headingsToScopes converts a flat list of (level, text) headings
// into a recursive Scope tree and reports the root level. The
// returned Sections list sits at the lowest level present in heads:
// for the historical "# ?\n## A\n## B" template that is level 1
// (the H1 wildcard wraps the H2 list); for a "## A\n## B" template
// it is level 2.
//
// Non-wildcard scopes are marked Closed=true so the
// heading-template's strict semantics survive: extra siblings at
// the same level produce diagnostics unless a "..." wildcard slot
// relaxes the position.
//
// The "..." text marks a slot scope at its position; the helper
// desugars it to the canonical wildcard matcher (`regex: '.+',
// repeat: { min: 0 }`).
func headingsToScopes(heads []FileHeading) ([]Scope, int, error) {
	rootLevel := 0
	for _, h := range heads {
		if rootLevel == 0 || h.Level < rootLevel {
			rootLevel = h.Level
		}
	}
	if rootLevel == 0 {
		return nil, 2, nil
	}

	type frame struct {
		scopes []Scope
		level  int
	}
	root := &frame{level: rootLevel - 1}
	stack := []*frame{root}

	for _, h := range heads {
		if h.Level < rootLevel {
			continue
		}
		for len(stack) > 1 && stack[len(stack)-1].level >= h.Level {
			parent := stack[len(stack)-2]
			top := stack[len(stack)-1]
			parent.scopes[len(parent.scopes)-1].Sections = top.scopes
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		sc, err := buildScopeFromHeading(h.Text)
		if err != nil {
			return nil, 0, err
		}
		parent.scopes = append(parent.scopes, sc)
		stack = append(stack, &frame{level: h.Level})
	}
	for len(stack) > 1 {
		parent := stack[len(stack)-2]
		top := stack[len(stack)-1]
		parent.scopes[len(parent.scopes)-1].Sections = top.scopes
		stack = stack[:len(stack)-1]
	}
	return root.scopes, rootLevel, nil
}

// buildScopeFromHeading translates a proto.md heading-row text into
// a Scope, desugaring the four template tokens listed in the
// section-schema reference.
func buildScopeFromHeading(text string) (Scope, error) {
	switch strings.TrimSpace(text) {
	case SectionWildcard:
		// `## ...` — any text, zero or more headings.
		return Scope{
			Heading: text,
			Matcher: &Matcher{
				Regex:  ".+",
				Repeat: Repeat{Set: true, Min: 0, Max: 0},
			},
			Closed: true,
		}, nil
	case "?":
		// `# ?` / `## ?` — any text, exactly one heading.
		return Scope{
			Heading: text,
			Matcher: &Matcher{Regex: ".+"},
			Closed:  true,
		}, nil
	}
	if fieldinterp.ContainsField(text) {
		regex := protoTokenRegex(text)
		if countDigitsHelpers(regex) > 1 {
			return Scope{}, fmt.Errorf(
				"heading %q: `{n}` may appear at most once "+
					"(the matcher reads a single named `n` capture)",
				text)
		}
		return Scope{
			Heading: text,
			Matcher: &Matcher{Regex: regex},
			Closed:  true,
		}, nil
	}
	return Scope{
		Heading: text,
		Matcher: &Matcher{Regex: regexp.QuoteMeta(text)},
		Closed:  true,
	}, nil
}

// protoTokenRegex desugars a heading-row text containing `{...}`
// tokens into a `regex:` body using the section-schema's helpers:
// `{n}` → `\#(digits)` (numeric capture) and `{field}` →
// `\#(fmvar(field))` (frontmatter lookup). The function preserves
// literal text in between via regexp.QuoteMeta so backslashes and
// regex metacharacters survive the round-trip.
func protoTokenRegex(text string) string {
	parts := fieldinterp.SplitOnFields(text)
	fields := fieldinterp.Fields(text)
	if len(parts) == 0 {
		return regexp.QuoteMeta(text)
	}
	var b strings.Builder
	for i, part := range parts {
		b.WriteString(regexp.QuoteMeta(part))
		if i < len(fields) {
			name := fields[i]
			if name == "n" {
				b.WriteString(`\#(digits)`)
			} else {
				b.WriteString(`\#(fmvar(`)
				b.WriteString(name)
				b.WriteString(`))`)
			}
		}
	}
	return b.String()
}

// headingText extracts the plain text content of a heading node.
func headingText(h *ast.Heading, source []byte) string {
	var buf strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, &buf)
	}
	return buf.String()
}

func writeNodeText(n ast.Node, source []byte, buf *strings.Builder) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	if _, ok := n.(*ast.CodeSpan); ok {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			writeNodeText(c, source, buf)
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, buf)
	}
}
