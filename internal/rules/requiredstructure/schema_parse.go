package requiredstructure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/runcache"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// schemaConfig holds the parsed schema frontmatter.
type schemaConfig struct {
	FrontMatterCUE string
	// FilenamePatterns are the globs the document basename must
	// match — the basename passes when it matches any one of them
	// (OR semantics, issue 817). A `<?require filename:?>` may spell
	// this as a single glob string or a YAML sequence of glob
	// strings. Empty means no filename constraint.
	FilenamePatterns []string

	// Frontmatter carries the per-key constraint expression
	// strings. Populated alongside FrontMatterCUE so the
	// SchemaDiagnostic emitter can render one diagnostic per CUE
	// error path with the source expression available for
	// "expected" extraction.
	Frontmatter map[string]string

	// FrontmatterLines records the 1-based line number of each
	// front-matter key in the schema source, when known. The
	// yaml.Node based parser populates this; the legacy
	// yaml.Unmarshal path leaves it empty.
	FrontmatterLines map[string]int

	// FrontmatterMeta captures plan 136's deprecation metadata when
	// the schema declares a field in the map form (`type:` +
	// `deprecated:` siblings). The key shape mirrors Frontmatter
	// (with the optional "?" suffix preserved) so the validator
	// can look up metadata using the same key it sees on the CUE
	// constraint.
	FrontmatterMeta map[string]schema.FieldMeta
}

// schemaHeading represents a required heading from the schema.
type schemaHeading struct {
	Level    int
	Text     string         // raw text, may contain {field} or ?
	compiled *regexp.Regexp // pre-compiled regex for {field} patterns; nil if not needed
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
	InBody   bool           // true if in body content, false if in heading
	BodyText string         // the full expected body line text with field substituted
	compiled *regexp.Regexp // pre-compiled pattern for BodyText; nil for non-body sync points
}

var cueIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const sectionWildcard = "..."

// parseSchemaFrontMatter extracts the schema configuration from
// frontmatter. cache is the optional RunCache the CUE compile uses
// to share its result across schemas with identical CUE source —
// the inner validateCUESchemaSyntax routes through it when non-nil.
// Tests pass nil to keep the parser standalone.
func parseSchemaFrontMatter(prefix []byte, cache *runcache.Cache) (schemaConfig, error) {
	cfg := schemaConfig{}
	if prefix == nil {
		return cfg, nil
	}
	yamlBytes := lint.FrontMatterYAML(prefix)
	derivedSchema, perKey, meta, err := deriveFrontMatterCUE(yamlBytes)
	if err != nil {
		return cfg, err
	}
	cfg.FrontMatterCUE = derivedSchema
	cfg.Frontmatter = perKey
	cfg.FrontmatterMeta = meta
	if err := validateCUESchemaSyntaxWith(cache, cfg.FrontMatterCUE); err != nil {
		return cfg, err
	}
	// Capture per-key source lines. The YAML body sits after the
	// opening "---\n" fence in the schema file, so line numbers
	// from yaml.Node are off by 1 relative to the schema source.
	node, nodeErr := yamlutil.UnmarshalNodeSafe(yamlBytes)
	if nodeErr == nil {
		lines := yamlutil.TopLevelMappingLines(&node, 1)
		// `extends:` is a schema directive (plan 135); strip it from
		// the per-key source-line map so MDS020 never tries to point
		// at the reserved key for a frontmatter validation error.
		delete(lines, "extends")
		cfg.FrontmatterLines = lines
	}
	return cfg, nil
}

// extractRequireDirective walks the schema AST for a <?require?> PI
// and parses its YAML body to extract constraints like filename. The
// `filename:` value may be a single glob string or a YAML sequence of
// glob strings (issue 817); both decode to the returned slice.
func extractRequireDirective(f *lint.File) ([]string, error) {
	var filenamePatterns []string
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		pi, ok := c.(*piparser.ProcessingInstruction)
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
		var params map[string]any
		if err := yamlutil.UnmarshalSafe(body, &params); err != nil {
			return nil, fmt.Errorf("invalid <?require?> directive: %w", err)
		}
		pats, err := schema.DecodeFilenameField(params["filename"])
		if err != nil {
			return nil, fmt.Errorf("invalid <?require?> directive: %w", err)
		}
		filenamePatterns = pats
		break
	}
	return filenamePatterns, nil
}

func deriveFrontMatterCUE(yamlBytes []byte) (string, map[string]string, map[string]schema.FieldMeta, error) {
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
		return "", nil, nil, fmt.Errorf("parsing schema frontmatter: %w", err)
	}
	// `extends:` is a reserved schema-engine directive (plan 135),
	// not a document-frontmatter constraint. Strip it before the
	// CUE expression is built so the legacy body-sync path does not
	// require documents to carry the literal `extends:` value.
	delete(raw, "extends")
	if err := schema.RejectProtoFrontmatterClosed(raw); err != nil {
		return "", nil, nil, err
	}
	if len(raw) == 0 {
		return "", nil, nil, nil
	}

	// Detach plan-136 metadata-form values so the CUE expression
	// only sees the embedded `type:` constraint. Without this the
	// `cueExprForMap` walker would emit the whole metadata map as a
	// struct constraint and reject any document value that isn't a
	// matching mapping.
	meta := map[string]schema.FieldMeta{}
	for k, v := range raw {
		expr, fm, isMeta, err := schema.ExtractFieldMeta(v)
		if err != nil {
			return "", nil, nil, fmt.Errorf("schema frontmatter %q: %w", k, err)
		}
		if !isMeta {
			continue
		}
		raw[k] = expr
		meta[k] = fm
	}

	expr, err := cueExprForMap(raw)
	if err != nil {
		return "", nil, nil, fmt.Errorf("parsing schema frontmatter constraints: %w", err)
	}
	perKey := make(map[string]string, len(raw))
	for k, v := range raw {
		ke, err := cueExprForValue(v)
		if err != nil {
			continue
		}
		perKey[k] = ke
	}
	if len(meta) == 0 {
		meta = nil
	}
	return "close(" + expr + ")", perKey, meta, nil
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

// piOpenPrefix / piClose are the literal markers
// collectBodySyncPoints scans for to skip processing-instruction
// blocks. They are package-level so the byte slices are allocated
// once rather than per call.
var (
	piOpenPrefix = []byte("<?")
	piClose      = []byte("?>")
	spaceSep     = []byte{' '}
)

// isPIOpenLine reports whether a raw body line opens a processing
// instruction, mirroring the block parser in pkg/markdown: at most
// three spaces of indentation, a `<?` opener, and a non-empty name
// (the bytes up to the first whitespace or `?>`). An indented code
// example showing a directive is therefore not mistaken for one.
func isPIOpenLine(raw []byte) bool {
	if astutil.CountLeadingSpaces(raw) > 3 {
		return false
	}
	trimmed := bytes.TrimLeft(raw, " ")
	trimmed = bytes.TrimRight(trimmed, " \t\r\n")
	if !bytes.HasPrefix(trimmed, piOpenPrefix) {
		return false
	}
	rest := trimmed[len(piOpenPrefix):]
	if len(rest) == 0 {
		return false
	}
	if rest[0] == ' ' || rest[0] == '\t' {
		return false
	}
	if bytes.HasPrefix(rest, piClose) {
		return false
	}
	return true
}

// maxSchemaIncludeDepth is the maximum nesting depth for schema includes.
const maxSchemaIncludeDepth = 10

// parseSchema reads schema bytes, extracts frontmatter config and
// required headings. When schemaPath is non-empty, <?include?>
// directives are expanded and their headings spliced in.
//
// Tests call parseSchema with no cache; the rule's hot path goes
// through parseSchemaWithCache so the inner CUE compile shares its
// result across schemas with the same CUE source. parseSchema is
// the canonical name kept for the existing test surface; the
// cache-aware form is a thin wrapper that adds the cache argument
// and surfaces the include set the cache wrapper needs for
// dependency tracking.
func parseSchema(data []byte, schemaPath string, maxBytes int64) (*parsedSchema, error) {
	sch, _, err := parseSchemaWithCache(data, schemaPath, maxBytes, nil)
	return sch, err
}

// parseSchemaWithCache is parseSchema with an optional RunCache wired
// through to validateCUESchemaSyntax so the CUE compile can be shared
// across schemas with identical CUE source. A nil cache restores the
// original behaviour (one fresh cuecontext per call).
//
// The returned includes slice carries every fragment path the
// schema's <?include?> directives reached. Paths are in the SAME
// coordinate system as schemaPath — typically project-relative when
// the rule's loadSchemaAt passes a project-relative schemaPath. The
// cache-aware wrapper (cachedParseSchema) anchors them onto the
// workspace root via absoluteIncludes before storing them on
// schemaParseResult, so RunCache.Invalidate can match against the
// absolute paths the LSP passes. Returns nil when schemaPath is
// empty (no filesystem identity → no include resolution).
//
// On a parseSchemaFrontMatter error (the common LSP editing state
// where the schema's CUE is mid-edit and fails CompileString), the
// function still surfaces a partial *parsedSchema carrying the
// derived FrontMatterCUE alongside the error. The rule's caller
// discards a non-nil sch when err != nil, but the cache wrapper
// reads schemaCUESources(sch) for invalidation tracking — so the
// failed CompiledCUE entry the build cached is evicted when the
// next Invalidate(schemaPath) fires.
func parseSchemaWithCache(
	data []byte, schemaPath string, maxBytes int64, cache *runcache.Cache,
) (*parsedSchema, []string, error) {
	return parseSchemaWithRootFS(data, schemaPath, maxBytes, cache, nil)
}

// parseSchemaWithRootFS is parseSchemaWithCache plus the workspace RootFS
// used to read <?include?> fragments. The Session/WASM and LSP engines
// have no usable OS disk (os.ReadFile is "not implemented on js" in
// WASM), so schema includes must read through the in-memory workspace FS;
// the CLI passes a nil rootFS and reads from disk. cachedParseSchema
// supplies f.RootFS.
func parseSchemaWithRootFS(
	data []byte, schemaPath string, maxBytes int64, cache *runcache.Cache, rootFS fs.FS,
) (*parsedSchema, []string, error) {
	prefix, content := lint.StripFrontMatter(data)

	cfg, err := parseSchemaFrontMatter(prefix, cache)
	if err != nil {
		// parseSchemaFrontMatter populates cfg.FrontMatterCUE
		// before the validation step, so the partial schema we
		// surface here carries the source that just failed to
		// compile. The cache wrapper then records it in
		// schemaCUESources so RunCache.Invalidate(schemaPath)
		// drops the failed CompiledCUE entry on the next edit.
		return &parsedSchema{Config: cfg}, nil, err
	}

	// lint.NewFile returns a nil error in every code path
	// (internal/lint/file.go); the defensive check the previous
	// signature carried was unreachable from any test. Discard the
	// error here so the patch carries no untestable defensive
	// branch.
	f, _ := lint.NewFile("schema", content)

	// Extract <?require?> directive from schema body.
	filenamePatterns, err := extractRequireDirective(f)
	if err != nil {
		// cfg.FrontMatterCUE was already compiled (and cached
		// via RunCache.CompiledCUE) before extractRequireDirective
		// ran, so surface a partial *parsedSchema so the cache
		// wrapper can record cueSources. Without this, an LSP
		// mid-edit state where <?require?> is malformed leaves
		// the CompiledCUE entry uncoupled from any schemaPath
		// and Invalidate(schemaPath) cannot evict it.
		return &parsedSchema{Config: cfg}, nil, err
	}
	cfg.FilenamePatterns = filenamePatterns

	// Extract headings, expanding <?include?> directives in the schema.
	var headings []docHeading
	var includes []string
	if schemaPath != "" {
		cleanPath := filepath.Clean(schemaPath)
		visited := map[string]struct{}{cleanPath: {}}
		chain := []string{cleanPath}
		var fp []string
		headings, fp, includes, err = extractSchemaHeadings(f, schemaPath, visited, chain, maxBytes, rootFS)
		if err != nil {
			// Surface a partial *parsedSchema (carrying the
			// frontmatter CUE source the cache will track via
			// schemaCUESources) AND the partial include set the
			// walk reached before erroring. The rule's caller
			// discards the partial schema on err, but the cache
			// wrapper records both pieces so a later
			// Invalidate(fragment) on a fixed include still
			// evicts the schema's failed-parse slot.
			return &parsedSchema{Config: cfg}, includes, err
		}
		if len(fp) > 0 && len(cfg.FilenamePatterns) == 0 {
			cfg.FilenamePatterns = fp
		}
	} else {
		headings = extractHeadings(f)
	}

	schHeadings := make([]schemaHeading, len(headings))
	syncPoints := make(map[int][]syncPoint)
	// fieldCache is scoped to this one parse: it dedupes repeated
	// {field} template text within this schema's headings and body,
	// then is discarded with the rest of this call's locals. See
	// fieldPatternCache's doc comment for why this isn't a package var;
	// the zero value costs nothing when the schema has no {field} text.
	var fieldCache fieldPatternCache

	for i, h := range headings {
		schHeadings[i] = buildSchemaHeading(h, &fieldCache)
		for _, f := range fieldinterp.Fields(h.Text) {
			syncPoints[i] = append(syncPoints[i], syncPoint{Field: f})
		}
	}

	collectBodySyncPoints(content, headings, syncPoints, &fieldCache)

	return &parsedSchema{
		Config:     cfg,
		Headings:   schHeadings,
		SyncPoints: syncPoints,
	}, includes, nil
}

// extractSchemaHeadings walks the schema AST, collecting headings and
// expanding <?include?> PIs by splicing in the included file's headings.
// It uses a visited set for cycle detection.
//
// The returned includes slice carries the resolved path of every
// fragment reached during the walk — both direct includes and
// transitively-included ones. Paths are in the SAME coordinate
// system as schemaPath: resolveSchemaIncludePath joins each include
// onto filepath.Dir(schemaPath), so a project-relative schemaPath
// produces project-relative include entries (and an absolute
// schemaPath produces absolute ones). cachedParseSchemaWith later
// anchors them onto absRoot via absoluteIncludes before storing on
// schemaParseResult, so RunCache.Invalidate's absolute keys match.
// Order is source order at the current level, with each fragment's
// transitive set appended after the fragment's own path.
func extractSchemaHeadings(
	schemaFile *lint.File, schemaPath string,
	visited map[string]struct{}, chain []string, maxBytes int64, rootFS fs.FS,
) ([]docHeading, []string, []string, error) {
	var headings []docHeading
	var filenamePatterns []string
	var includes []string

	err := ast.Walk(schemaFile.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			text := headingText(node, schemaFile.Source)
			line := schemaFile.LineOfOffset(node.Lines().At(0).Start)
			headings = append(headings, docHeading{Level: node.Level, Text: text, Line: line})

		case *piparser.ProcessingInstruction:
			if node.Name != "include" {
				return ast.WalkContinue, nil
			}
			fragHeadings, fp, fragIncluded, subIncludes, walkErr := expandSchemaInclude(
				node, schemaFile.Source, schemaPath, visited, chain, maxBytes, rootFS)
			if walkErr != nil {
				// Record the broken-but-known fragment plus any
				// transitive sub-includes the recursive walk
				// reached before erroring. The outer
				// extractSchemaHeadings surfaces this partial
				// `includes` slice up to parseSchemaWithCache so
				// the cache wrapper's reverse-include index covers
				// the full dependency footprint, broken edges
				// included. Without this, Invalidate(fragment) on
				// a fix would not evict the schema's failed-parse
				// slot.
				if fragIncluded != "" {
					includes = append(includes, fragIncluded)
				}
				includes = append(includes, subIncludes...)
				return ast.WalkStop, walkErr
			}
			if len(fp) > 0 && len(filenamePatterns) == 0 {
				filenamePatterns = fp
			}
			headings = append(headings, fragHeadings...)
			if fragIncluded != "" {
				includes = append(includes, fragIncluded)
			}
			includes = append(includes, subIncludes...)
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		// Surface the partial include set so the cache wrapper can
		// still register dependents on fragments the walk reached
		// before erroring. A schema that fails to parse because of
		// a downstream <?include?> issue must still let
		// Invalidate(brokenFragment) evict the schema's failed-parse
		// slot when the fragment is fixed.
		return nil, nil, includes, err
	}

	return headings, filenamePatterns, includes, nil
}

// resolveSchemaIncludePath extracts and validates the file parameter from
// an include PI, returning the resolved filesystem path.
func resolveSchemaIncludePath(
	pi *piparser.ProcessingInstruction, source []byte, schemaPath string,
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

// expandSchemaInclude resolves and recursively expands a schema
// include PI. It returns the fragment's headings, any filename
// pattern declared on the fragment, the fragment's resolved path
// (in the same coordinate system as schemaPath — typically
// project-relative; cachedParseSchemaWith anchors it onto absRoot
// before storing), the fragment's transitive include set, and an
// error. The path-plus-subIncludes pair lets the caller record the
// full dependency footprint on RunCache's reverse-include index.
func expandSchemaInclude(
	pi *piparser.ProcessingInstruction, source []byte,
	schemaPath string, visited map[string]struct{}, chain []string, maxBytes int64, rootFS fs.FS,
) ([]docHeading, []string, string, []string, error) {
	includedPath, err := resolveSchemaIncludePath(pi, source, schemaPath)
	if err != nil {
		return nil, nil, "", nil, err
	}

	// Surface includedPath on every error past this point so the
	// caller can register the broken-but-known fragment in
	// RunCache's reverse-include index. When the user fixes the
	// fragment (creates the missing file, breaks the cycle, raises
	// the depth limit), Invalidate(fragmentAbs) then evicts this
	// schema's failed-parse slot and the next Check re-parses.
	if len(chain) > maxSchemaIncludeDepth {
		return nil, nil, includedPath, nil, fmt.Errorf(
			"schema include depth exceeds maximum (%d)", maxSchemaIncludeDepth)
	}
	if _, ok := visited[includedPath]; ok {
		chainCopy := make([]string, len(chain), len(chain)+1)
		copy(chainCopy, chain)
		chainCopy = append(chainCopy, includedPath)
		return nil, nil, includedPath, nil, fmt.Errorf(
			"cyclic include: %s", strings.Join(chainCopy, " -> "))
	}

	fragData, err := readSchemaInclude(rootFS, includedPath, maxBytes)
	if err != nil {
		return nil, nil, includedPath, nil, fmt.Errorf(
			"cannot read schema include file %q: %w", includedPath, err)
	}

	_, fragContent := lint.StripFrontMatter(fragData)
	// lint.NewFile returns a nil error in every code path
	// (internal/lint/file.go); the previous defensive branch was
	// unreachable from any test. Discard the error so the patch
	// carries no untestable defensive code.
	fragFile, _ := lint.NewFile(includedPath, fragContent)

	fp, err := extractRequireDirective(fragFile)
	if err != nil {
		return nil, nil, includedPath, nil, err
	}

	visited[includedPath] = struct{}{}
	chain = append(chain, includedPath)
	fragHeadings, fp2, subIncludes, err := extractSchemaHeadings(
		fragFile, includedPath, visited, chain, maxBytes, rootFS)
	delete(visited, includedPath)
	if err != nil {
		// Surface includedPath plus whatever subIncludes the
		// recursive walk reached so the full dependency
		// footprint (broken fragment + every successful one
		// above it) is recorded in the cache's reverse-include
		// index.
		return nil, nil, includedPath, subIncludes, err
	}
	if len(fp2) > 0 && len(fp) == 0 {
		fp = fp2
	}

	return fragHeadings, fp, includedPath, subIncludes, nil
}

// extractPIFileParam parses the YAML body of an include PI to extract
// the "file" parameter.
func extractPIFileParam(pi *piparser.ProcessingInstruction, source []byte) (string, error) {
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

	var params map[string]string
	if err := yamlutil.UnmarshalSafe([]byte(body), &params); err != nil {
		return "", fmt.Errorf("invalid include directive YAML: %w", err)
	}

	return params["file"], nil
}

// readSchemaFile reads the schema file using the file's RootFS when available,
// falling back to os.ReadFile. When RootFS is set, absolute and
// parent-traversal paths are rejected to prevent reading outside the
// project root.
func readSchemaFile(f *lint.File, schema string) ([]byte, error) {
	if f.RootFS != nil {
		// Reject absolute paths and parent traversals.
		if filepath.IsAbs(schema) {
			return nil, fmt.Errorf("absolute schema path not allowed")
		}
		clean := filepath.ToSlash(filepath.Clean(schema))
		clean = strings.TrimPrefix(clean, "./")
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("schema path %q escapes project root", schema)
		}
		return bytelimit.ReadFSFileLimited(f.RootFS, clean, f.MaxInputBytes)
	}
	return bytelimit.ReadFileLimited(schema, f.MaxInputBytes)
}

// schemaRootFS returns f's workspace RootFS, or nil when f is nil. It is
// the seam cachedParseSchema uses to feed RootFS-based <?include?>
// fragment reads (see parseSchemaWithRootFS).
func schemaRootFS(f *lint.File) fs.FS {
	if f == nil {
		return nil
	}
	return f.RootFS
}

// readSchemaInclude reads a schema <?include?> fragment. It prefers the
// workspace RootFS — the Session/WASM and LSP path, where os.ReadFile is
// absent or "not implemented on js" — and falls back to the OS filesystem
// for the on-disk CLI. includedPath is project-relative
// (resolveSchemaIncludePath rejects ".."), so it is a valid fs.FS key.
func readSchemaInclude(rootFS fs.FS, includedPath string, maxBytes int64) ([]byte, error) {
	if rootFS != nil {
		return bytelimit.ReadFSFileLimited(rootFS, filepath.ToSlash(includedPath), maxBytes)
	}
	return bytelimit.ReadFileLimited(includedPath, maxBytes)
}
